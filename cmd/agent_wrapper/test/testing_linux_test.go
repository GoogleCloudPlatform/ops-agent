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

package testing_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

const bytesWritten = 50
const testRepeats = 50

var commandPath string = ""

func getPrintCommand(bytes int) []string {
	return []string{
		filepath.Join(getBinaryPackagePath(), "test", "linux_print.bash"),
		fmt.Sprint(bytes)}
}

func getPrintHandleCommand(bytes int) []string {
	return []string{
		filepath.Join(getBinaryPackagePath(), "test", "linux_print_handle.bash"),
		fmt.Sprint(bytes)}
}

func getBinaryPackagePath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filename))
}

func agentWrapCommand(logPath string, arguments []string) *exec.Cmd {
	args := []string{"-log_path", logPath}
	args = append(args, arguments...)
	return exec.Command(commandPath, args...)
}

func runCommandWithSIGTERM(dir string, args []string) (int, error) {
	logFile := filepath.Join(dir, "log.txt")
	cmd := agentWrapCommand(logFile, args)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() {
		time.Sleep(1 * time.Second)
		// This has the side effect that tests must run in at most 80 seconds
		cmd.Process.Signal(syscall.SIGTERM)
	}()

	cmdErr := cmd.Wait()
	info, err := os.Stat(logFile)
	if err != nil {
		return 0, nil
	}

	return int(info.Size()), cmdErr
}

func runRepeat[V any](t *testing.T, times int, toRun func(string) V) []V {
	results := []V{}
	m := sync.Mutex{}
	wg := sync.WaitGroup{}
	for i := 0; i < times; i += 1 {
		tempDir := t.TempDir()
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := toRun(tempDir)
			m.Lock()
			results = append(results, out)
			m.Unlock()
		}()
		time.Sleep(5 * time.Millisecond)
	}
	wg.Wait()
	return results
}

func TestHandledPrintsAllOrNone(t *testing.T) {
	type outputType struct {
		size int
		err  error
	}

	result := runRepeat(t, testRepeats, func(dir string) outputType {
		out, err := runCommandWithSIGTERM(dir, getPrintHandleCommand(bytesWritten))
		return outputType{out, err}
	})

	for _, v := range result {
		if 0 < v.size && v.size < bytesWritten {
			t.Errorf("Expected log to have %v bytes, got %v", bytesWritten, v.size)
		} else if v.size != 0 && v.err != nil {
			t.Error(v.err)
		} else if v.size == 0 && v.err != nil {
			log.Printf(
				"Got error with no output %q."+
					" This can happen if we signal agent_wrapper before it sets up its signal handling",
				v.err)
		}
	}

}

func TestNonHandledPrintsPartialOrNone(t *testing.T) {
	type outputType struct {
		size int
		err  error
	}

	result := runRepeat(t, testRepeats, func(dir string) outputType {
		out, err := runCommandWithSIGTERM(dir, getPrintCommand(bytesWritten))
		return outputType{out, err}
	})

	for _, v := range result {
		if v.size == bytesWritten {
			t.Errorf("Expected log to less than %v bytes, got %v", bytesWritten, v.size)
		} else if v.size != 0 && (v.err == nil || v.err.Error() != "exit status 255") {
			t.Errorf("Expected exit status of 255, got error %v", v.err)
		} else if v.size == 0 && v.err != nil {
			log.Printf(
				"Got error with no output %q."+
					" This can happen if we signal agent_wrapper before it sets up its signal handling",
				v.err)
		}
	}

}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	commandPath = filepath.Join(dir, "agent_wrap")
	buildCmd := exec.Command("go", "build", "-o", commandPath, getBinaryPackagePath())
	buildCmd.Stderr = os.Stderr
	buildCmd.Stdout = os.Stdout

	if err := buildCmd.Run(); err != nil {
		log.Fatalf("Could not run %v, got error %v", buildCmd, err)
	}
	os.Exit(m.Run())
}
