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

package confgenerator_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

func TestWriteConfigFile(t *testing.T) {
	// The parent directories do not exist yet.
	path := filepath.Join(t.TempDir(), "a", "b", "test.conf")

	if err := confgenerator.WriteConfigFile([]byte("first"), path); err != nil {
		t.Fatal(err)
	}
	checkContent(t, path, "first\n")

	// Writing to the same path again replaces the previous content.
	if err := confgenerator.WriteConfigFile([]byte("second"), path); err != nil {
		t.Fatal(err)
	}
	checkContent(t, path, "second\n")
}

func TestWriteConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not supported on Windows")
	}
	dir := filepath.Join(t.TempDir(), "out")
	path := filepath.Join(dir, "test.conf")

	if err := confgenerator.WriteConfigFile([]byte("content"), path); err != nil {
		t.Fatal(err)
	}
	checkPermissions(t, dir, 0700)
	checkPermissions(t, path, 0600)
}

func TestWriteConfigFileKeepsExistingDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not supported on Windows")
	}
	dir := t.TempDir()
	// Only directories created by WriteConfigFile get their permissions set.
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := confgenerator.WriteConfigFile([]byte("content"), filepath.Join(dir, "test.conf")); err != nil {
		t.Fatal(err)
	}
	checkPermissions(t, dir, 0755)
}

func TestWriteConfigFileReplacesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not supported on Windows")
	}
	path := filepath.Join(t.TempDir(), "test.conf")

	// Start with a file that was written with different permissions.
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Chmod explicitly, since the mode passed to WriteFile is subject to umask.
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	// Keep the existing file open to tell whether it gets replaced by a new
	// file or is overwritten in place.
	existing, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer existing.Close()

	if err := confgenerator.WriteConfigFile([]byte("new"), path); err != nil {
		t.Fatal(err)
	}
	checkContent(t, path, "new\n")
	checkPermissions(t, path, 0600)

	got, err := io.ReadAll(existing)
	if err != nil {
		t.Fatal(err)
	}
	if want := "old\n"; string(got) != want {
		t.Errorf("existing file was overwritten in place: got content %q, want %q", got, want)
	}
}

func TestGenerateFilesFromConfigPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not supported on Windows")
	}
	pc := linuxTestPlatform
	ctx := pc.platform.TestContext(context.Background())
	// The user config does not exist, so the built-in config is used as is.
	uc, err := confgenerator.MergeConfFiles(ctx, filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, service := range []string{"fluentbit", "otel"} {
		t.Run(service, func(t *testing.T) {
			outDir := filepath.Join(t.TempDir(), service)
			if err := uc.GenerateFilesFromConfig(ctx, service, pc.defaultLogsDir, pc.defaultStateDir, outDir); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(outDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 {
				t.Fatalf("no files were generated in %s", outDir)
			}
			for _, entry := range entries {
				checkPermissions(t, filepath.Join(outDir, entry.Name()), 0600)
			}
		})
	}
}

func checkContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s has content %q, want %q", path, got, want)
	}
}

func checkPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has permissions %04o, want %04o", path, got, want)
	}
}
