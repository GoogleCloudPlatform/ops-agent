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

package healthchecks_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"gotest.tools/v3/assert"
)

var (
	TestFailure = healthchecks.HealthCheckError{
		Code:         "TestFailure",
		Class:        healthchecks.Generic,
		Message:      "",
		Action:       "",
		ResourceLink: "",
		IsFatal:      true,
	}

	TestWarning = healthchecks.HealthCheckError{
		Code:         "TestWarning",
		Class:        healthchecks.Generic,
		Message:      "",
		Action:       "",
		ResourceLink: "",
		IsFatal:      false,
	}
)

func generateExpectedResultMessage(name string, result string) string {
	return fmt.Sprintf("[%s] Result: %s", name, result)
}

type FailureCheck struct{}

func (c FailureCheck) Name() string {
	return "Failure Check"
}

func (c FailureCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return TestFailure
}

func TestCheckFailure(t *testing.T) {
	wantMessage := ""
	wantAction := ""
	testCheck := FailureCheck{}
	testLogger, _ := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}

	err := testCheck.RunCheck(testLogger, testGCEResource)

	assert.ErrorIs(t, err, TestFailure)
	healthError, _ := err.(healthchecks.HealthCheckError)
	assert.Equal(t, wantMessage, healthError.Message)
	assert.Equal(t, wantAction, healthError.Action)
}

type WarningCheck struct{}

func (c WarningCheck) Name() string {
	return "Warning Check"
}

func (c WarningCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return TestWarning
}

func TestCheckWarning(t *testing.T) {
	wantMessage := ""
	wantAction := ""
	testCheck := WarningCheck{}
	testLogger, _ := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}

	err := testCheck.RunCheck(testLogger, testGCEResource)

	assert.ErrorIs(t, err, TestWarning)
	healthError, _ := err.(healthchecks.HealthCheckError)
	assert.Equal(t, wantMessage, healthError.Message)
	assert.Equal(t, wantAction, healthError.Action)
}

type SuccessCheck struct{}

func (c SuccessCheck) Name() string {
	return "Success Check"
}

func (c SuccessCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return nil
}

func TestCheckSuccess(t *testing.T) {
	testCheck := SuccessCheck{}
	testLogger, _ := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}
	err := testCheck.RunCheck(testLogger, testGCEResource)

	assert.NilError(t, err)
}

type ErrorCheck struct{}

func (c ErrorCheck) Name() string {
	return "Error Check"
}

func (c ErrorCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return errors.New("Test error.")
}

func TestCheckError(t *testing.T) {
	wantMessage := "Test error."
	testCheck := ErrorCheck{}
	testLogger, _ := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}

	err := testCheck.RunCheck(testLogger, testGCEResource)

	assert.ErrorContains(t, err, wantMessage)
}

func TestRunAllHealthChecks(t *testing.T) {
	fCheck := FailureCheck{}
	wCheck := WarningCheck{}
	sCheck := SuccessCheck{}
	eCheck := ErrorCheck{}
	allHealthChecks := healthchecks.HealthCheckRegistry{fCheck, wCheck, sCheck, eCheck}
	testLogger, observedLogs := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}
	allCheckResults := allHealthChecks.RunAllHealthChecks(testLogger, testGCEResource)

	var expected string
	var result string
	var level string
	for idx, r := range allCheckResults {
		switch r.Name {
		case "Error Check":
			result = "ERROR"
			level = "error"
		case "Success Check":
			result = "PASS"
			level = "info"
		case "Warning Check":
			result = "WARNING"
			level = "warn"
		case "Failure Check":
			result = "FAIL"
			level = "error"
		}
		expected = generateExpectedResultMessage(r.Name, result)

		assert.Check(t, strings.Contains(observedLogs.All()[idx].Entry.Message, expected))
		assert.Equal(t, observedLogs.All()[idx].Entry.Level.String(), level)
	}
}

type MultipleFailureResultCheck struct{}

func (c MultipleFailureResultCheck) Name() string {
	return "MultipleResult Check"
}

func (c MultipleFailureResultCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return errors.Join(nil, errors.New("Test error."), TestWarning, TestFailure)
}

func TestMultipleFailureResultCheck(t *testing.T) {
	mCheck := MultipleFailureResultCheck{}
	wantErrorMessage := "Test error."
	expectedError := generateExpectedResultMessage(mCheck.Name(), "ERROR")
	expectedWarning := generateExpectedResultMessage(mCheck.Name(), "WARNING")
	expectedFailure := generateExpectedResultMessage(mCheck.Name(), "FAIL")
	testLogger, observedLogs := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}

	err := mCheck.RunCheck(testLogger, testGCEResource)
	result := healthchecks.HealthCheckResult{Name: mCheck.Name(), Err: err}
	result.LogResult(testLogger)

	assert.ErrorContains(t, err, wantErrorMessage)
	assert.ErrorIs(t, err, TestWarning)
	assert.ErrorIs(t, err, TestFailure)

	assert.Check(t, strings.Contains(observedLogs.All()[0].Entry.Message, expectedError))
	assert.Equal(t, observedLogs.All()[0].Entry.Level.String(), "error")
	assert.Check(t, strings.Contains(observedLogs.All()[1].Entry.Message, expectedWarning))
	assert.Equal(t, observedLogs.All()[1].Entry.Level.String(), "warn")
	assert.Check(t, strings.Contains(observedLogs.All()[2].Entry.Message, expectedFailure))
	assert.Equal(t, observedLogs.All()[2].Entry.Level.String(), "error")
}

type MultipleSuccessResultCheck struct{}

func (c MultipleSuccessResultCheck) Name() string {
	return "MultipleResult Check"
}

func (c MultipleSuccessResultCheck) RunCheck(logger logs.StructuredLogger, resource resourcedetector.Resource) error {
	return errors.Join(nil, nil, nil)
}

func TestMultipleSuccessResultCheck(t *testing.T) {
	sCheck := MultipleSuccessResultCheck{}
	expectedSuccess := generateExpectedResultMessage(sCheck.Name(), "PASS")
	testLogger, observedLogs := logs.DiscardLogger()
	testGCEResource := resourcedetector.GCEResource{}

	err := sCheck.RunCheck(testLogger, testGCEResource)
	result := healthchecks.HealthCheckResult{Name: sCheck.Name(), Err: err}
	result.LogResult(testLogger)

	assert.NilError(t, err)

	assert.Check(t, strings.Contains(observedLogs.All()[0].Entry.Message, expectedSuccess))
	assert.Equal(t, observedLogs.All()[0].Entry.Level.String(), "info")
}
