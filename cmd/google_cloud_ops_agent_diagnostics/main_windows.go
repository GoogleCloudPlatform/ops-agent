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

//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"go.opentelemetry.io/otel"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

const (
	DiagnosticsEventID      uint32 = 2
	ERROR_SUCCESS           uint32 = 0
	ERROR_FILE_NOT_FOUND    uint32 = 2
	ERROR_INVALID_DATA      uint32 = 13
	ERROR_INVALID_PARAMETER uint32 = 87
)

type service struct {
	log      debug.Log
	userConf string
}

func (s *service) Handle(err error) {
	s.log.Error(DiagnosticsEventID, fmt.Sprintf("error collecting metrics: %v", err))
}

func run() error {
	name := "google-cloud-ops-agent-diagnostics"
	elog, err := eventlog.Open(name)
	if err != nil {
		// probably futile
		return err
	}
	defer elog.Close()

	elog.Info(DiagnosticsEventID, fmt.Sprintf("starting %s service", name))
	err = svc.Run(name, &service{log: elog})
	if err != nil {
		elog.Error(DiagnosticsEventID, fmt.Sprintf("%s service failed: %v", name, err))
		return err
	}
	elog.Info(DiagnosticsEventID, fmt.Sprintf("%s service stopped", name))
	return nil
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	if err := s.parseFlags(args); err != nil {
		s.log.Error(DiagnosticsEventID, fmt.Sprintf("failed to parse arguments: %v", err))
		return false, ERROR_INVALID_PARAMETER
	}

	userUc, mergedUc, err := getUserAndMergedConfigs(s.userConf, "windows")
	if err != nil {
		s.log.Error(DiagnosticsEventID, fmt.Sprintf("failed to obtain unified configuration: %v", err))
		return false, ERROR_FILE_NOT_FOUND
	}
	s.log.Info(DiagnosticsEventID, "obtained unified configuration")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	death := make(chan bool)

	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()

	go func() {
		// Manage windows service signals
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
				case svc.Stop, svc.Shutdown:
					death <- true
					return
				default:
					s.log.Error(DiagnosticsEventID, fmt.Sprintf("unexpected control request #%d", c))
				}
			}
		}
	}()

	// Set otel error handler
	otel.SetErrorHandler(s)

	err = self_metrics.CollectOpsAgentSelfMetrics(userUc, mergedUc, death)
	if err != nil {
		s.log.Error(DiagnosticsEventID, fmt.Sprintf("failed to collect ops agent self metrics: %v", err))
		return false, ERROR_INVALID_DATA
	}

	return false, ERROR_SUCCESS
}

func (s *service) parseFlags(args []string) error {
	s.log.Info(DiagnosticsEventID, fmt.Sprintf("args: %#v", args))
	var fs flag.FlagSet
	fs.StringVar(&s.userConf, "config", "", "path to the user specified agent config")
	s.log.Info(DiagnosticsEventID, s.userConf)

	allArgs := append([]string{}, os.Args[1:]...)
	allArgs = append(allArgs, args[1:]...)
	return fs.Parse(allArgs)
}
