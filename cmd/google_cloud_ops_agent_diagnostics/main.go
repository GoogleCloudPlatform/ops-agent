// Copyright 2020 Google LLC
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
	"flag"
	"log"
	"os"
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var (
	config = flag.String("config", "/etc/google-cloud-ops-agent/config.yaml", "path to the user specified agent config")
)

type service struct {
	log          debug.Log
	userConf     string
}

func main() {
	flag.Parse()
	if ok, err := svc.IsWindowsService(); ok && err == nil {
		if err := run(); err != nil {
			log.Fatal(err)
		}
	}
}

func run() error {
	name := "google-cloud-ops-agent-diagnostics"
        elog, err := eventlog.Open(name)
	if err != nil {
		// probably futile
		return err
	}
	defer elog.Close()
	
	elog.Info(1, fmt.Sprintf("starting %s service", name))
        err = svc.Run(name, &service{log: elog})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return err
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", name))
	return nil
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	if err := s.parseFlags(args); err != nil {
		s.log.Error(1, fmt.Sprintf("failed to parse arguments: %v", err))
		// ERROR_INVALID_ARGUMENT
		return false, 0x00000057
	}
	uc, err := s.generateConfigs()
	if err != nil {
		s.log.Error(1, fmt.Sprintf("failed to generate config files: %v", err))
		// 2 is "file not found"
		return false, 2
	}
	s.log.Info(1, "generated configuration files")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	death := make(chan bool)

	defer func() {
		changes <- svc.Status{State: svc.StopPending}
	}()

	go func() {
	waitForSignal:
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
				case svc.Stop, svc.Shutdown:
					death <- true
					break waitForSignal
				default:
					s.log.Error(1, fmt.Sprintf("unexpected control request #%d", c))
				}
			}
		}
	}()

	err = self_metrics.CollectOpsAgentSelfMetrics(&uc, death)
	if err != nil {
		return false, 0
	}

	return
}

func (s *service) parseFlags(args []string) error {
	s.log.Info(1, fmt.Sprintf("args: %#v", args))
	var fs flag.FlagSet
	fs.StringVar(&s.userConf, "config", "", "path to the user specified agent config")

	allArgs := append([]string{}, os.Args[1:]...)
	allArgs = append(allArgs, args[1:]...)
	return fs.Parse(allArgs)
}

func (s *service) generateConfigs() (confgenerator.UnifiedConfig, error) {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	builtInConfig, mergedConfig, err := confgenerator.MergeConfFiles(s.userConf, "windows", apps.BuiltInConfStructs)
	if err != nil {
		return confgenerator.UnifiedConfig{}, err
	}

	s.log.Info(1, fmt.Sprintf("Built-in config:\n%s", builtInConfig))
	s.log.Info(1, fmt.Sprintf("Merged config:\n%s", mergedConfig))
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(mergedConfig, "windows")
	if err != nil {
		return confgenerator.UnifiedConfig{}, err
	}
	return uc, nil
}

