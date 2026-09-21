// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	EngineEventID uint32 = 1
	StdoutEventID uint32 = 2
)

type service struct {
	log          debug.Log
	userConf     string
	outDirectory string
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	allArgs := append([]string{}, os.Args[1:]...)
	allArgs = append(allArgs, args[1:]...)
	if err := s.parseFlags(allArgs); err != nil {
		s.log.Error(EngineEventID, fmt.Sprintf("failed to parse arguments: %v", err))
		// ERROR_INVALID_ARGUMENT
		return false, 0x00000057
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	if err := s.startSubagents(); err != nil {
		s.log.Error(EngineEventID, fmt.Sprintf("failed to start subagents: %v", err))
		// TODO: Ignore failures for partial startup?
	}
	s.log.Info(EngineEventID, "started subagents")
	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				return
			default:
				s.log.Error(EngineEventID, fmt.Sprintf("unexpected control request #%d", c))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *service) parseFlags(args []string) error {
	s.log.Info(EngineEventID, fmt.Sprintf("args: %#v", args))
	var fs flag.FlagSet
	fs.StringVar(&s.userConf, "in", "", "path to the user specified agent config")
	fs.StringVar(&s.outDirectory, "out", "", "directory to write generated configuration files to")
	return fs.Parse(args)
}

func (s *service) startSubagents() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	handle, err := manager.OpenService(otelServiceDescription.name)
	if err != nil {
		// service not found?
		return err
	}
	defer handle.Close()
	if err := handle.Start(); err != nil {
		// TODO: Should we be ignoring failures for partial startup?
		if !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			s.log.Error(EngineEventID, fmt.Sprintf("failed to start %q: %v", otelServiceDescription.name, err))
		}
	}
	return nil
}

type eventLogWriter struct {
	EventID  uint32
	EventLog *eventlog.Log
}

func (w *eventLogWriter) Write(p []byte) (int, error) {
	err := w.EventLog.Info(w.EventID, string(p))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func run(name string) error {
	elog, err := eventlog.Open(name)
	if err != nil {
		// probably futile
		return err
	}
	defer elog.Close()

	// Redirect stdout to the event log to capture internal messages
	log.SetOutput(&eventLogWriter{
		EventID:  StdoutEventID,
		EventLog: elog,
	})

	elog.Info(1, fmt.Sprintf("starting %s service", name))
	err = svc.Run(name, &service{log: elog})
	if err != nil {
		elog.Error(EngineEventID, fmt.Sprintf("%s service failed: %v", name, err))
		return err
	}
	elog.Info(EngineEventID, fmt.Sprintf("%s service stopped", name))
	return nil
}
