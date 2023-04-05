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

type HealthCheckResult struct {
	Name string
	Err  error
}

func singleErrorResultMessage(e error, Name string) string {
	if e != nil {
		if healthError, ok := e.(HealthCheckError); ok {
			return fmt.Sprintf("%s - Result: FAIL, Error code: %s, Failure: %s, Solution: %s, Resource: %s",
				Name, healthError.Code, healthError.Message, healthError.Action, healthError.ResourceLink)
		}
		return fmt.Sprintf("%s - Result: ERROR, Detail: %s", Name, e.Error())
	}
	return fmt.Sprintf("%s - Result: PASS", Name)
}

func (r HealthCheckResult) String() string {
	var message string
	if mwErr, ok := r.Err.(MultiWrappedError); ok {
		for _, e := range mwErr.Unwrap() {
			message = message + "\n" + singleErrorResultMessage(e, r.Name)
		}
	} else {
		message = singleErrorResultMessage(r.Err, r.Name)
	}
	return message
}

type HealthCheckRegistry []HealthCheck

func HealthCheckRegistryFactory() HealthCheckRegistry {
	return HealthCheckRegistry{
		PortsCheck{},
		NetworkCheck{},
		APICheck{},
	}
}

func CreateHealthChecksLogger(logDir string) (logger *log.Logger, closer func()) {
	path := filepath.Join(logDir, healthChecksLogFile)
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("failed to create directory for %q: %v", path, err)
		return log.Default(), func() {}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("failed to open health checks log file %q: %v", path, err)
		return log.Default(), func() {}
	}

	return log.New(file, "", log.Ldate|log.Ltime|log.Lshortfile), func() { file.Close() }
}

func (r HealthCheckRegistry) RunAllHealthChecks(logger *log.Logger) []HealthCheckResult {
	result := []HealthCheckResult{}

	for _, c := range r {
		err := c.RunCheck(logger)

		r := HealthCheckResult{Name: c.Name(), Err: err}
		logger.Println(r)
		result = append(result, r)
	}

	return result
}

func LogHealthCheckResults(healthCheckResults []HealthCheckResult, infoLogger func(string), errorLogger func(string)) {
	for _, result := range healthCheckResults {
		if result.Err != nil {
			errorLogger(result.String())
		} else {
			infoLogger(result.String())
		}
	}
}
