// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "fmt"

// CollectionError is a map of errors. It is used for operations
// where you want to complete the full set of operations, i.e. something
// on a full directory, and collect the results.
type CollectionError map[string]error

// Error implements the error interface. It formats the error
// with the intention of being output to stderr.
func (e CollectionError) Error() string {
	msg := ""
	for name, err := range e {
		combinedErr := fmt.Errorf("%s: %w", name, err)
		msg += fmt.Sprintf("%v\n", combinedErr)
	}
	return msg
}

// ExitCodeError is a wrapped error that, if propogated to the main
// function of the program, will cause the program to exit with the
// specified exit code.
type ExitCodeError struct {
	exitCode int
	err      error
}

// Error implements the error interface.
func (e *ExitCodeError) Error() string {
	return e.err.Error()
}

// wrapExitCodeError wraps an existing error with an exit code.
func wrapExitCodeError(exitCode int, err error) error {
	return &ExitCodeError{
		exitCode: exitCode,
		err:      err,
	}
}
