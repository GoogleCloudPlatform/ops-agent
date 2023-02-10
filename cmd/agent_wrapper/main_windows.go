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
// +build windows

package main

import (
	"os/exec"
	"unsafe"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"golang.org/x/sys/windows"
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

	return cmd.Run()
}
