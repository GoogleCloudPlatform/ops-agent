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

//go:build !windows
// +build !windows

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
)

var (
	config = flag.String("config", "/etc/google-cloud-ops-agent/config.yaml", "path to the user specified agent config")
)

func run() error {
	userUc, mergedUc, err := getUserAndMergedConfigs(*config, "linux")
	if err != nil {
		return err
	}

	death := make(chan bool)

	go func() {
		osSignal := make(chan os.Signal, 1)
		signal.Notify(osSignal, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)

		// Wait for stop signal
		for {
			select {
			case <-osSignal:
				death <- true
				return
			}
		}
	}()

	err = self_metrics.CollectOpsAgentSelfMetrics(userUc, mergedUc, death)
	if err != nil {
		return err
	}

	return nil
}
