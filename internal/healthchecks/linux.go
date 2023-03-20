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

package healthchecks

import (
	"errors"
	"net"
	"os/exec"
	"syscall"
)

func isSubagentActive(subagent string) (bool, error) {
	_, err := exec.Command("systemctl", "is-active", "--quiet", subagent).Output()
	if err != nil {
		// if the service doesn't exist return false with no error
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ProcessState.ExitCode() == 3 {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func isPortUnavailableError(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	if errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	return false
}

func isConnectionRefusedError(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
