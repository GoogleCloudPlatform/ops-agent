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
	"fmt"
	"io"
	"os/exec"
	"unsafe"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

func getMergedConfig(userConfPath string) (*confgenerator.UnifiedConfig, error) {
	return getMergedConfigForPlatform(userConfPath, "windows")
}

func configureJob() (*windows.Handle, error) {
	jobHandle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		jobHandle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(jobHandle)
		return nil, err
	}

	err = windows.AssignProcessToJobObject(jobHandle, windows.CurrentProcess())

	if err != nil {
		windows.CloseHandle(jobHandle)
		return nil, err
	}

	return &jobHandle, nil
}

func runCommand(cmd *exec.Cmd) error {
	handle, err := configureJob()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(*handle)

	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isService {
		return runAsService(cmd)
	}
	return cmd.Run()
}

func runAsService(cmd *exec.Cmd) error {
	name := "google-cloud-ops-agent-abba"
	elog, err := eventlog.Open(name)
	if err != nil {
		// probably futile
		return err
	}
	defer elog.Close()
	return svc.Run(name, &service{elog, cmd})
}

const (
	DiagnosticsEventID      uint32 = 2
	ERROR_SUCCESS           uint32 = 0
	ERROR_FILE_NOT_FOUND    uint32 = 2
	ERROR_INVALID_DATA      uint32 = 13
	ERROR_INVALID_PARAMETER uint32 = 87
)

type service struct {
	log     debug.Log
	command *exec.Cmd
}

func (s *service) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	pipeOut, pipeIn := io.Pipe()
	s.command.Stdin = pipeOut
	defer func() {
		changes <- svc.Status{State: svc.StopPending}
		pipeIn.Close()
		pipeOut.Close()
		s.command.Wait()
	}()
	s.command.Start()
	// Manage windows service signals
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				return false, ERROR_SUCCESS
			default:
				s.log.Error(DiagnosticsEventID, fmt.Sprintf("unexpected control request #%d", c))
			}
		}
	}
}
