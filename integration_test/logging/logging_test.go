package logging

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
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
			logger.Close()

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

	// This will result in an error because /etc is already a directory.
	invalidPath := "../../../../../../../../../etc"
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

	for i := 0; i < 50; i++ {
		testName := fmt.Sprintf("shard_%d", i)
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			dirLog.ToFile(testName).Print(testName)

			content, err := os.ReadFile(path.Join(tmp, testName))
			if err != nil {
				t.Fatalf("could not read file %v: %v", testName, err)
			}
			if !strings.Contains(string(content), testName) {
				t.Errorf("file %v did not contain expected content %q. Instead was: %q", testName, testName, string(content))
			}
		})
	}
}
