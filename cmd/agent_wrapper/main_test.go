package main

import (
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

type testCase struct {
	beforeLogSizes   []int
	expectedLogSizes []int
	bytesWritten     int8
	config           string
}

func (tc *testCase) getLogFile(dir string) string {
	return path.Join(dir, logFileName)
}
func (tc *testCase) getConfigPath(dir string) string {
	return path.Join(dir, "config.yaml")
}
func (tc *testCase) makeConfig(dir string) error {
	file, err := os.OpenFile(tc.getConfigPath(dir), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(tc.config)
	return err
}

func (tc *testCase) makeLogFileSizes(dir string) error {
	if len(tc.beforeLogSizes) == 0 {
		return nil
	}
	if err := makeLogFile(path.Join(dir, logFileName), tc.beforeLogSizes[0]); err != nil {
		return err
	}
	for i, size := range tc.beforeLogSizes[1:] {
		t := time.UnixMicro(int64(len(tc.beforeLogSizes)-i) * 1000 * 1000 * 60 * 60 * 24)
		name := logFileName + "-" + t.Format(timeFormat)
		if err := makeLogFile(path.Join(dir, name), size); err != nil {
			return err
		}
	}
	return nil
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

func getLogFileSizes(dir string) ([]int, error) {
	files, err := os.ReadDir(dir)
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

func makeLogFile(path string, size int) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Truncate(int64(size))
}

func runTest(t *testing.T, tc testCase) {
	dir := t.TempDir()
	// Set up config
	if err := tc.makeConfig(dir); err != nil {
		t.Error(err)
		return
	}

	// Set up folder with pre-made log files
	if err := tc.makeLogFileSizes(dir); err != nil {
		t.Error(err)
		return
	}

	// Run command to print specific amount of bytes
	if err := run(tc.getLogFile(dir), tc.getConfigPath(dir), tc.getCommand()); err != nil {
		t.Error(err)
		return
	}

	// Compare log files in directory sorted by rotation to expected sizes
	gotLogSizes, err := getLogFileSizes(dir)
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(tc.expectedLogSizes, gotLogSizes) {
		t.Errorf("Test %v: Expected log sizes %v, got %v", tc, tc.expectedLogSizes, gotLogSizes)
	}
}

func TestAgentWrapper(t *testing.T) {
	cases := []testCase{
		{
			beforeLogSizes:   []int{},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config:           "",
		},
		{
			beforeLogSizes:   []int{0},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config:           "",
		},
		{
			beforeLogSizes:   []int{1},
			expectedLogSizes: []int{11},
			bytesWritten:     10,
			config:           "",
		},
		{
			beforeLogSizes:   []int{400 * megabyte},
			expectedLogSizes: []int{10, 400 * megabyte},
			bytesWritten:     10,
			config:           "",
		},
		{
			beforeLogSizes:   []int{399 * megabyte},
			expectedLogSizes: []int{399*megabyte + 10},
			bytesWritten:     10,
			config:           "",
		},
		{
			beforeLogSizes:   []int{megabyte},
			expectedLogSizes: []int{10, megabyte},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1`,
		},
		{
			beforeLogSizes:   []int{megabyte, megabyte + 1},
			expectedLogSizes: []int{10, megabyte},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1`,
		},
		{
			beforeLogSizes:   []int{megabyte, megabyte + 1},
			expectedLogSizes: []int{10, megabyte, megabyte + 1},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    max_file_size_megabytes: 1
    backup_count: 2`,
		},
		{
			beforeLogSizes:   []int{megabyte},
			expectedLogSizes: []int{megabyte + 10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false
    max_file_size_megabytes: 1`,
		},
		{
			beforeLogSizes:   []int{},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
		{
			beforeLogSizes:   []int{0},
			expectedLogSizes: []int{10},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
		{
			beforeLogSizes:   []int{1},
			expectedLogSizes: []int{11},
			bytesWritten:     10,
			config: `global:
  default_self_log_file_rotation:
    enabled: false`,
		},
	}
	for _, tc := range cases {
		runTest(t, tc)
	}
}
