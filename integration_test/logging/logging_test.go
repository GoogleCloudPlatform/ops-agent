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

// Unit tests for the DirectoryLogger, which is itself only used
// for integration testing.

package logging

import (
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Tests normal use of the DirectoryLogger: ToFile() correctly dispatches
// log content into the appropriate files.
func TestDirectoryLogger(t *testing.T) {
	testCases := []struct {
		Name       string
		LogContent map[string]string
	}{
		{
			Name: "NoContent",
		},
		{
			Name:       "MainLogOnly",
			LogContent: map[string]string{mainLogName: "main_stuff"},
		},
		{
			Name: "MainLogAndFooLog",
			LogContent: map[string]string{
				mainLogName: "main_stuff",
				"foo.log":   "foo_stuff",
			},
		},
		{
			Name:       "SubdirLog",
			LogContent: map[string]string{"subdir/bar.log": "subdir_stuff"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			dir := t.TempDir()
			logger, err := NewDirectoryLogger(dir)
			if err != nil {
				t.Fatal(err)
			}
			for path, content := range testCase.LogContent {
				if path == mainLogName {
					logger.ToMainLog().Print(content)
				} else {
					logger.ToFile(path).Print(content)
				}
			}
			if err := logger.Close(); err != nil {
				t.Errorf("logger.Close() failed with err=%v", err)
			}

			// Now verify that the expected content ended up on disk.
			for file, expectedContent := range testCase.LogContent {
				if expectedContent != "" {
					if actualContent, err := os.ReadFile(path.Join(dir, file)); err != nil {
						t.Errorf("could not read file %v: %v", file, err)
					} else {
						if !strings.Contains(string(actualContent), expectedContent) {
							t.Errorf("file %v did not contain expected content %q. Instead was: %q", file, expectedContent, string(actualContent))
						}
					}
				}
			}
		})
	}
}

func TestInvalidDirectory(t *testing.T) {
	_, err := NewDirectoryLogger("")
	if err == nil {
		t.Error(`NewDirectoryLogger(""), got nil error, wanted non-nil`)
	}
}

// Tests that when errors happen in ToFile(), it degrades gracefully to
// log.Default().
func TestInvalidFile(t *testing.T) {
	tmp := t.TempDir()
	dirLog, err := NewDirectoryLogger(tmp)
	if err != nil {
		t.Fatalf("NewDirectoryLogger(%q) failed: %v", tmp, err)
	}

	defer func() {
		if err := dirLog.Close(); err != nil {
			t.Errorf("dirLog.Close() failed with err=%v", err)
		}
	}()

	// This will result in an error because /etc is already a directory.
	invalidPath := "../../../../../../../../../etc"

	// This check is necessary only because this test is bundled into the other
	// unit tests (which run on both linux and windows). In practice, the
	// DirectoryLogger is only run on linux. Future work could be to figure out
	// how to set up github workflows/actions to separate out this test so that
	// it's only run when changes are made to the DirectoryLogger itself, and
	// only on linux (meaning this check could be deleted).
	if runtime.GOOS == "windows" {
		// In this case, C:/Users is already a directory.
		invalidPath = "C:/Users"
	}
	logger := dirLog.ToFile(invalidPath)
	if logger != log.Default() {
		t.Errorf("ToFile(%q) = %p, want %p (AKA log.Default())", invalidPath, logger, log.Default())
	}
}

func TestConcurrentLogging(t *testing.T) {
	tmp := t.TempDir()
	dirLog, err := NewDirectoryLogger(tmp)
	if err != nil {
		t.Fatalf("NewDirectoryLogger(%q) failed: %v", tmp, err)
	}

	defer func() {
		if err := dirLog.Close(); err != nil {
			t.Errorf("dirLog.Close() failed with err=%v", err)
		}
	}()

	var wg sync.WaitGroup
	limit := 50
	for i := 0; i < limit; i++ {
		testName := fmt.Sprintf("shard_%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dirLog.ToFile(testName).Print(testName)
		}()
	}
	wg.Wait()
	if err := dirLog.Close(); err != nil {
		t.Errorf("dirLog.Close() failed with err=%v", err)
	}
	for i := 0; i < limit; i++ {
		testName := fmt.Sprintf("shard_%d", i)
		content, err := os.ReadFile(path.Join(tmp, testName))
		if err != nil {
			t.Fatalf("could not read file %v: %v", testName, err)
		}
		if !strings.Contains(string(content), testName) {
			t.Errorf("file %v did not contain expected content %q. Instead was: %q", testName, testName, string(content))
		}
	}
}
