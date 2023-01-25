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

package healthchecks

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var healthChecksLogFile = "health-checks.log"

type HealthCheck interface {
	Name() string
	RunCheck(logger *log.Logger) error
}

type HealthCheckRegistry []HealthCheck

func createHealthChecksLogger(logDir string) (*log.Logger, error) {
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(filepath.Dir(logDir), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for %q: %w", logDir, err)
	}
	file, err := os.OpenFile(filepath.Join(logDir, healthChecksLogFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile), nil
}

func (r HealthCheckRegistry) RunAllHealthChecks(logDir string) (map[string]string, error) {
	var message string
	result := map[string]string{}

	logger, err := createHealthChecksLogger(logDir)
	if err != nil {
		return result, err
	}

	for _, c := range r {
		err := c.RunCheck(logger)
		if err != nil {
			if healthError, ok := err.(HealthCheckError); ok {
				message = fmt.Sprintf("%s - Result: FAIL, ERROR_CODE: %s, Failure: %s, Solution: %s",
					c.Name(), healthError.Code, healthError.Message, healthError.Action)
			} else {
				message = fmt.Sprintf("%s - Result: ERROR, Detail: %s", c.Name(), err.Error())
			}
		} else {
			message = fmt.Sprintf("%s - Result: PASS", c.Name())
		}
		logger.Print(message)
		result[c.Name()] = message
	}

	return result, nil
}
