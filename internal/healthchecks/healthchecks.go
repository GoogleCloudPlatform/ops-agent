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
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
)

var healthChecksLogFile = "health-checks.log"

type HealthCheck interface {
	Name() string
	RunCheck(logger logs.StructuredLogger) error
}

type HealthCheckResult struct {
	Name string
	Err  error
}

func singleErrorResultMessage(e error, Name string) string {
	if e != nil {
		if healthError, ok := e.(HealthCheckError); ok {
			return fmt.Sprintf("[%s] Result: FAIL, Error code: %s, Failure: %s, Solution: %s, Resource: %s",
				Name, healthError.Code, healthError.Message, healthError.Action, healthError.ResourceLink)
		}
		return fmt.Sprintf("[%s] Result: ERROR, Detail: %s", Name, e.Error())
	}
	return fmt.Sprintf("[%s] Result: PASS", Name)
}

func (r HealthCheckResult) LogResult(logger logs.StructuredLogger) {
	for _, m := range r.StringSlice() {
		if r.Err == nil {
			logger.Infof(m)
		} else {
			logger.Errorf(m)
		}
	}
}

func (r HealthCheckResult) StringSlice() []string {
	if mwErr, ok := r.Err.(MultiWrappedError); ok {
		var messageList []string
		for _, e := range mwErr.Unwrap() {
			messageList = append(messageList, singleErrorResultMessage(e, r.Name))
		}
		return messageList
	}
	return []string{singleErrorResultMessage(r.Err, r.Name)}
}

func (r HealthCheckResult) String() string {
	return strings.Join(r.StringSlice(), "\n")
}

func LogHealthCheckResults(healthCheckResults []HealthCheckResult, logger logs.StructuredLogger) {
	for _, result := range healthCheckResults {
		result.LogResult(logger)
	}
}

func CreateHealthChecksLogger(logDir string) logs.StructuredLogger {
	path := filepath.Join(logDir, healthChecksLogFile)
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("failed to create directory for %q: %v", path, err)
		return logs.Default()
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("failed to open health checks log file %q: %v", path, err)
		return logs.Default()
	}
	file.Close()

	return logs.New(path)
}

type HealthCheckRegistry []HealthCheck

func HealthCheckRegistryFactory() HealthCheckRegistry {
	return HealthCheckRegistry{
		PortsCheck{},
		NetworkCheck{},
		APICheck{},
	}
}

func (r HealthCheckRegistry) RunAllHealthChecks(logger logs.StructuredLogger) []HealthCheckResult {
	var result []HealthCheckResult

	for _, c := range r {
		r := HealthCheckResult{Name: c.Name(), Err: c.RunCheck(logger)}
		r.LogResult(logger)
		result = append(result, r)
	}
	return result
}
