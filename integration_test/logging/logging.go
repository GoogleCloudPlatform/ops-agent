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

/*
Package logging has utilities to aid in recording logs for tests.
*/
package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/multierr"
)

// DirectoryLogger manages a set of log.Loggers that each write to a different file
// within a single directory. To write to a file named foo.txt in that directory,
// call dirLog.ToFile("foo.txt") and use the log.Logger that it returns.
// The ToMainLog method is a shorthand that logs to a file called main_log.txt.
//
// It is the caller's responsibility to call Close() when done with a DirectoryLogger.
// After calling Close(), the DirectoryLogger and the log.Loggers it returns should
// not be used anymore.
type DirectoryLogger struct {
	Directory string

	mutex     sync.Mutex
	loggers   map[string]*log.Logger
	openFiles []*os.File
}

// NewDirectoryLogger returns a new DirectoryLogger managing the files in the given
// directory.
func NewDirectoryLogger(dir string) (*DirectoryLogger, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, fmt.Errorf("NewDirectoryLogger() could not create dir %v for writing: %v", dir, err)
	}
	logger := &DirectoryLogger{
		Directory: dir,
		loggers:   make(map[string]*log.Logger),
	}
	return logger, nil
}

const (
	mainLogName = "main_log.txt"
)

// ToMainLog returns a Logger that writes to the main log (main_log.txt).
func (dirLog *DirectoryLogger) ToMainLog() *log.Logger {
	return dirLog.ToFile(mainLogName)
}

// ToFile returns a Logger that writes to a file with the given path
// inside the directory managed by this DirectoryLogger.
func (dirLog *DirectoryLogger) ToFile(file string) *log.Logger {
	dirLog.mutex.Lock()
	defer dirLog.mutex.Unlock()
	if logger, ok := dirLog.loggers[file]; ok {
		return logger
	}

	fullPath := filepath.Join(dirLog.Directory, file)

	// Handle a value of 'file' with slashes in it by creating the necessary subdirectories.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0777); err != nil {
		log.Printf("Failed to create parent directories to log to %v (%v), dumping to backup instead", fullPath, err)
		return log.Default()
	}

	f, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open %v for logging (%v), dumping to backup instead", fullPath, err)
		return log.Default()
	}
	dirLog.openFiles = append(dirLog.openFiles, f)

	logger := log.New(f, "", log.LstdFlags)
	dirLog.loggers[file] = logger
	return logger
}

// Close closes all open files that have been used for logging so far.
func (dirLog *DirectoryLogger) Close() error {
	var err error
	for _, f := range dirLog.openFiles {
		err = multierr.Append(err, f.Close())
	}
	// Reset the DirectoryLogger, to avoid confusing errors if somebody does end
	// up using the DirectoryLogger after Close()ing it (which they are not
	// supposed to do).
	dirLog.loggers = make(map[string]*log.Logger)
	dirLog.openFiles = nil
	return err
}
