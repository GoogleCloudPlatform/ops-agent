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
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func getMergedConfig(userConfPath string) (*confgenerator.UnifiedConfig, error) {
	return getMergedConfigForPlatform(userConfPath, "linux")
}

func handleSignals(cmd *exec.Cmd) {
	sigs := make(chan os.Signal, 1)
	for {
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGTERM)
		sig := <-sigs
		cmd.Process.Signal(sig)
		if sig == syscall.SIGTERM {
			time.Sleep(80 * time.Second)
			log.Fatalf("Wrapped %v did not terminate in 80 seconds\n", cmd.Path)
		}
	}
}

func runCommand(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	go handleSignals(cmd)
	return cmd.Run()
}
