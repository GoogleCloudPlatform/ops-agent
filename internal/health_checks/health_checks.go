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

package health_checks

import (
	"fmt"
	"log"
    "time"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"go.uber.org/multierr"
)

type HealthCheck interface {
	RunCheck() error
	Fail(failureCode string)
	Error(err error)
	Log(message string)
    GetResult() string
	GetCheckLog() string
	GetFailureMessage() string
	GetActionMessage() string
}

type healthCheckRegistry struct {
	environment    string
	healthCheckMap map[string]HealthCheck
}

func (r healthCheckRegistry) RegisterCheck(name string, c HealthCheck) {
	r.healthCheckMap[name] = c
}

var GCEHealthChecks = &healthCheckRegistry{
	environment:    "GCE",
	healthCheckMap: make(map[string]HealthCheck),
}

type BaseHealthCheck struct {
	HealthCheck
    errored         bool
	failed          bool
    err             error          
    failure         HealthCheckFailure
	checkLog        string
	failureMessage  string
	solutionMessage string
}

func NewHealthCheck() HealthCheck {
	return &BaseHealthCheck{
		failed:          false,
        errored:         false,
        err:             nil,
        failure:         HealthCheckFailure{},
		checkLog:        "",
		failureMessage:  "",
		solutionMessage: "",
	}
}

func (b *BaseHealthCheck) RunCheck() error {
	return nil
}

func (b *BaseHealthCheck) Fail(failureCode string) {
	b.failed = true
    fail, err := GetFailure(failureCode)
    if err != nil {
        b.Error(err)
    }
    b.failure = fail
}

func (b *BaseHealthCheck) Error(err error) {
    // TODO : What to do with error ?
    b.err = err
    b.errored = true
	b.Fail("health-check-error")
}

func (b *BaseHealthCheck) Log(message string) {
	b.checkLog = time.Now().Format("[2006-01-02 15:04:05]") + " " + message + "\n" + b.checkLog
}

func (b *BaseHealthCheck) GetCheckLog() string {
	return b.checkLog
}

func (b *BaseHealthCheck) GetFailureMessage() string {
	return b.failure.message
}

func (b *BaseHealthCheck) GetActionMessage() string {
	return b.failure.action
}

func (b *BaseHealthCheck) GetResult() string {
	if b.errored {
        return "ERROR"
    }
    if b.failed {
		return "FAIL"
	} else {
		return "PASS"
	}
}

func getGCEMetadata() (resourcedetector.GCEResource, error) {
	MetadataResource, err := resourcedetector.GetResource()
	if err != nil {
		return resourcedetector.GCEResource{}, fmt.Errorf("can't get resource metadata: %w", err)
	}
	if gceMetadata, ok := MetadataResource.(resourcedetector.GCEResource); ok {
		log.Printf("gceMetadata : %+v", gceMetadata)
		return gceMetadata, nil
	} else {
		return resourcedetector.GCEResource{}, fmt.Errorf("not in GCE")
	}
}

func RunAllHealthChecks(uc *confgenerator.UnifiedConfig) (string, error) {
	var multiErr error
	var result []string
	result = append(result, "========================================")
	result = append(result, "Health Checks : ")
	for name, c := range GCEHealthChecks.healthCheckMap {
		err := c.RunCheck()
		if err != nil {
			multiErr = multierr.Append(multiErr, err)
		}
		result = append(result, fmt.Sprintf("Check: %s, Result: %s", name, c.GetResult()))
		result = append(result, fmt.Sprintf("Failure: %s", c.GetFailureMessage()))
		result = append(result, fmt.Sprintf("Solution: %s ", c.GetActionMessage()))
        result = append(result, fmt.Sprintf("Log: %s \n", c.GetCheckLog()))
	}
	result = append(result, "===========================================================")

	return strings.Join(result, "\n"), multiErr
}
