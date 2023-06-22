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

package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const MaximumWaitForProcessStart = 5 * time.Second

func handleSignals(cmd *exec.Cmd) {
	// Relay signals that should be passed down to the subprocess we are wrapping.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGCONT)
	for {
		sig := <-sigs
		start := time.Now()
		// It is possible that we receive a signal before the code for `cmd.Run()` set cmd.Process.
		// In this case we wait up to MaximumWaitForProcessStart before giving up relaying the signal.
		for {
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
				break
			} else if time.Until(start.Add(MaximumWaitForProcessStart)) <= 0 {
				// We waited MaximumWaitForProcessStart and the subprocess did not start. Give up on relaying the signal.
				log.Printf("Failed to relay signal %v to subprocess as it has not started yet", sig)
				break
			}
			time.Sleep(50 * time.Millisecond)
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
