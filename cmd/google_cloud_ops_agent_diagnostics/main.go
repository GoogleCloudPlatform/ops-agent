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
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/GoogleCloudPlatform/ops-agent/apps"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/self_metrics"
	"github.com/shirou/gopsutil/host"
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	// TODO(lingshi) Move this to a shared place across Linux and Windows.
	builtInConfig, mergedConfig, err := confgenerator.MergeConfFiles(*input, "linux", apps.BuiltInConfStructs)
	if err != nil {
		return err
	}

	// Log the built-in and merged config files to STDOUT. These are then written
	// by journald to var/log/syslog and so to Cloud Logging once the ops-agent is
	// running.
	log.Printf("Built-in config:\n%s", builtInConfig)
	log.Printf("Merged config:\n%s", mergedConfig)

	hostInfo, err := host.Info()
	if err != nil {
		return err
	}
	uc, err := confgenerator.ParseUnifiedConfigAndValidate(mergedConfig, hostInfo.OS)
	if err != nil {
		return fmt.Errorf("The agent config file is not valid. Detailed error: %s", err)
	}

	death := make(chan bool)

	go func() {
		osSignal := make(chan os.Signal, 1)
		signal.Notify(osSignal, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)

	waitForSignal:
		for {
			select {
			case <-osSignal:
				death <- true
				break waitForSignal
			}
		}
	}()

	err = self_metrics.CollectOpsAgentSelfMetrics(&uc, death)
	if err != nil {
		return err
	}

	return nil
}
