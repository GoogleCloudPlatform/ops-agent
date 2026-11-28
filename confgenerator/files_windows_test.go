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

package confgenerator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"golang.org/x/sys/windows"
)

// parentEntry is how an access control list shows the entry that the test adds
// to the parent directory.
const parentEntry = ";BU)"

func TestRestrictOutputDirAccess(t *testing.T) {
	// Add an entry to the access control list of the parent directory. The
	// directories and files that get created in it inherit that entry.
	parent := t.TempDir()
	setAccessControlList(t, parent, "D:(A;OICI;FRFX;;;BU)")
	dir := filepath.Join(parent, "out")
	// A file that already exists when access to the directory gets restricted.
	existing := filepath.Join(dir, "a", "existing.conf")
	if err := os.MkdirAll(filepath.Dir(existing), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("existing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if acl := accessControlList(t, existing); !strings.Contains(acl, parentEntry) {
		t.Fatalf("%s is expected to start out with the entry of the parent directory, got %s", existing, acl)
	}

	if err := confgenerator.RestrictOutputDirAccess(dir); err != nil {
		t.Fatal(err)
	}

	// A file that is written after access to the directory was restricted.
	created := filepath.Join(dir, "b", "created.conf")
	if err := confgenerator.WriteConfigFile([]byte("created"), created); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Dir(existing), existing, filepath.Dir(created), created} {
		if acl := accessControlList(t, path); strings.Contains(acl, parentEntry) {
			t.Errorf("%s still has the entry of the parent directory: %s", path, acl)
		}
	}
	// The current user keeps access to the files.
	for path, want := range map[string]string{existing: "existing\n", created: "created\n"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s has content %q, want %q", path, got, want)
		}
	}
}

func TestRestrictOutputDirAccessCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")

	if err := confgenerator.RestrictOutputDirAccess(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
}

// setAccessControlList replaces the entries of the access control list of path
// that are not inherited with the ones in sddl.
func setAccessControlList(t *testing.T, path, sddl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

// accessControlList returns the access control list of path in its string
// format.
func accessControlList(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
