package main

import (
	"context"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	timeFormat  = "2006-01-02T15-04-05.000"
	logFileName = "log_output_file"
	megabyte    = 1024 * 1024
)

type testSetUp struct {
	beforeLogSizes []int
	config         string
	dir            string
}

func setUp(t *testing.T, beforeLogSizes []int, config string) (*testSetUp, error) {
	setUp := testSetUp{
		beforeLogSizes: beforeLogSizes,
		config:         config,
		dir:            t.TempDir(),
	}
	// Set up config
	if err := setUp.makeConfig(); err != nil {
		return nil, err
	}

	// Set up folder with pre-made log files
	if err := setUp.makeLogFileSizes(); err != nil {
		return nil, err
	}
	return &setUp, nil
}

func (ts *testSetUp) getLogFile() string {
	return path.Join(ts.dir, logFileName)
}
func (ts *testSetUp) getConfigPath() string {
	return path.Join(ts.dir, "config.yaml")
}
func (ts *testSetUp) makeConfig() error {
	file, err := os.OpenFile(ts.getConfigPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(ts.config)
	return err
}

func (ts *testSetUp) makeLogFileSizes() error {
	if len(ts.beforeLogSizes) == 0 {
		return nil
	}
	if err := makeLogFile(ts.getLogFile(), ts.beforeLogSizes[0]); err != nil {
		return err
	}
	for i, size := range ts.beforeLogSizes[1:] {
		t := time.UnixMicro(int64(len(ts.beforeLogSizes)-i) * 1000 * 1000 * 60 * 60 * 24)
		name := logFileName + "-" + t.Format(timeFormat)
		if err := makeLogFile(path.Join(ts.dir, name), size); err != nil {
			return err
		}
	}
	return nil
}

func makeLogFile(path string, size int) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Truncate(int64(size))
}

type logFileType struct {
	time time.Time
	size int
}

func logFilesToSizes(logFiles []logFileType) []int {
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].time.Unix() > logFiles[j].time.Unix()
	})
	sizes := []int{}
	for _, f := range logFiles {
		sizes = append(sizes, f.size)
	}
	return sizes
}

func (ts *testSetUp) getLogFileSizes() ([]int, error) {
	files, err := os.ReadDir(ts.dir)
	if err != nil {
		return nil, err
	}
	logFiles := []logFileType{}

	for _, f := range files {
		if f.IsDir() || !strings.HasPrefix(f.Name(), logFileName) {
			continue
		}
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		ts := time.Now()
		if f.Name() != logFileName {
			ts, err = time.Parse(timeFormat, f.Name()[len(logFileName)+1:])
			if err != nil {
				return nil, err
			}
		}

		logFiles = append(logFiles, logFileType{ts, int(info.Size())})
	}

	return logFilesToSizes(logFiles), nil
}

type testCase struct {
	beforeLogSizes   []int
	expectedLogSizes []int
	bytesWritten     int
	config           string
}

func TestAgentWrapper(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name             string
		beforeLogSizes   []int
		expectedLogSizes []int
		bytesWritten     int
		config           string
	}{
		{
			name:             "write to empty dir",
			beforeLogSizes:   []int{},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config:           "",
		},
		{
			name:             "write to empty log file",
			beforeLogSizes:   []int{0},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config:           "",
		},
		{
			name:             "write to non-empty log file",
			beforeLogSizes:   []int{1},
			expectedLogSizes: []int{11},
			bytesWritten:     10,
			config:           "",
		},
		{
			name:             "write to full log file",
			beforeLogSizes:   []int{400 * megabyte},
			expectedLogSizes: []int{10, 400 * megabyte},
			bytesWritten:     10,
			config:           "",
		},
		{
			name:             "write to almost full log file",
			beforeLogSizes:   []int{399 * megabyte},
			expectedLogSizes: []int{399*megabyte + 10},
			bytesWritten:     10,
			config:           "",
		},
		{
			name:             "write to full log file w/ custom max_size",
			beforeLogSizes:   []int{megabyte},
			expectedLogSizes: []int{10, megabyte},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1`,
		},
		{
			name:             "default backup count of 1 is respected",
			beforeLogSizes:   []int{megabyte, megabyte + 1},
			expectedLogSizes: []int{10, megabyte},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1`,
		},
		{
			name:             "backup count of 2 is respected",
			beforeLogSizes:   []int{megabyte, megabyte + 1},
			expectedLogSizes: []int{10, megabyte, megabyte + 1},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1
    backup_count: 2`,
		},
		{
			name:             "rotation can be disabled",
			beforeLogSizes:   []int{megabyte},
			expectedLogSizes: []int{megabyte + 10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false
    max_file_size_megabytes: 1`,
		},
		{
			name:             "write to empty dir w/o rotation",
			beforeLogSizes:   []int{},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
		{
			name:             "write to empty log file w/o rotation",
			beforeLogSizes:   []int{0},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
		{
			name:             "write to non-empty log file",
			beforeLogSizes:   []int{1},
			expectedLogSizes: []int{11},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts, err := setUp(t, tc.beforeLogSizes, tc.config)
			if err != nil {
				t.Fatal(err)
			}

			// Run command to print specific amount of bytes
			if err := run(context.Background(), ts.getLogFile(), ts.getConfigPath(), getCommand(tc.bytesWritten)); err != nil {
				t.Fatal(err)
			}

			// Compare log files in directory sorted by rotation to expected sizes
			gotLogSizes, err := ts.getLogFileSizes()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tc.expectedLogSizes, gotLogSizes) {
				t.Fatalf("Test %v: Expected log sizes %v, got %v", tc, tc.expectedLogSizes, gotLogSizes)
			}
		})
	}
}
