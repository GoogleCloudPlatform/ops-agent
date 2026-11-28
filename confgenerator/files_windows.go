// Copyright 2026 Google LLC
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

package confgenerator

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// RestrictOutputDirAccess creates dir if it does not exist yet, and limits
// access to dir, and to the files and directories in it, to the current user,
// the SYSTEM account and the Administrators group.
//
// Permission bits are not supported on Windows, so the files written by
// WriteConfigFile get their access restrictions from the directory that they
// are written to instead.
func RestrictOutputDirAccess(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}
	// Allow full access (FA) for the SYSTEM account (SY) and the Administrators
	// group (BA). The access applies to dir itself, and is inherited by the
	// files (OI) and directories (CI) in it.
	sddl := "D:(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("failed to look up the current user: %w", err)
	}
	if !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		sddl += fmt.Sprintf("(A;OICI;FA;;;%s)", user.User.Sid)
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("failed to build the access control list for %q: %w", dir, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("failed to build the access control list for %q: %w", dir, err)
	}
	// PROTECTED_DACL_SECURITY_INFORMATION makes this access control list the
	// only one that applies to dir. The files and directories that already
	// exist in dir inherit it as well.
	if err := windows.SetNamedSecurityInfo(
		dir,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	); err != nil {
		return fmt.Errorf("failed to restrict access to %q: %w", dir, err)
	}
	return nil
}
