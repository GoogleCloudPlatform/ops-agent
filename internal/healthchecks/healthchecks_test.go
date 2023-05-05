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

	"github.com/GoogleCloudPlatform/ops-agent/internal/healthchecks"
	"github.com/GoogleCloudPlatform/ops-agent/internal/logs"
	"gotest.tools/v3/assert"
)

func generateExpectedResultMessage(name string, result string) string {
	return fmt.Sprintf("[%s] Result: %s", name, result)
}

type FailureCheck struct{}

func (c FailureCheck) Name() string {
	return "Failure Check"
}

func (c FailureCheck) RunCheck(logger *logs.FileLogger) error {
	return healthchecks.HcFailureErr
}

func TestCheckFailure(t *testing.T) {
	wantMessage := "The Health Check encountered an internal error."
	wantAction := "Submit a support case from Google Cloud console."
	testCheck := FailureCheck{}
	testLogger := logs.DiscardLogger()
	err := testCheck.RunCheck(testLogger)

	assert.ErrorIs(t, err, healthchecks.HcFailureErr)
	healthError, _ := err.(healthchecks.HealthCheckError)
	assert.Equal(t, wantMessage, healthError.Message)
	assert.Equal(t, wantAction, healthError.Action)
}

type SuccessCheck struct{}

func (c SuccessCheck) Name() string {
	return "Success Check"
}

func (c SuccessCheck) RunCheck(logger *logs.FileLogger) error {
	return nil
}

func TestCheckSuccess(t *testing.T) {
	testCheck := SuccessCheck{}
	testLogger := logs.DiscardLogger()

	err := testCheck.RunCheck(testLogger)

	assert.NilError(t, err)
}

type ErrorCheck struct{}

func (c ErrorCheck) Name() string {
	return "Error Check"
}

func (c ErrorCheck) RunCheck(logger *logs.FileLogger) error {
	return errors.New("Test error.")
}

func TestCheckError(t *testing.T) {
	wantMessage := "Test error."
	testCheck := ErrorCheck{}
	testLogger := logs.DiscardLogger()

	err := testCheck.RunCheck(testLogger)

	assert.ErrorContains(t, err, wantMessage)
}

func TestRunAllHealthChecks(t *testing.T) {
	fCheck := FailureCheck{}
	sCheck := SuccessCheck{}
	eCheck := ErrorCheck{}
	allHealthChecks := healthchecks.HealthCheckRegistry{fCheck, sCheck, eCheck}
	testLogger := logs.DiscardLogger()

	allCheckResults := allHealthChecks.RunAllHealthChecks(testLogger)

	var expected string
	var result string
	for _, r := range allCheckResults {
		switch r.Name {
		case "Error Check":
			result = "ERROR"
		case "Success Check":
			result = "PASS"
		case "Failure Check":
			result = "FAIL"
		}
		expected = generateExpectedResultMessage(r.Name, result)
		assert.Check(t, strings.Contains(r.String(), expected))
	}
}

type MultipleFailureResultCheck struct{}

func (c MultipleFailureResultCheck) Name() string {
	return "MultipleResult Check"
}

func (c MultipleFailureResultCheck) RunCheck(logger *logs.FileLogger) error {
	return errors.Join(nil, errors.New("Test error."), healthchecks.HcFailureErr)
}

func TestMultipleFailureResultCheck(t *testing.T) {
	mCheck := MultipleFailureResultCheck{}
	wantErrorMessage := "Test error."
	expectedFailure := generateExpectedResultMessage(mCheck.Name(), "FAIL")
	expectedError := generateExpectedResultMessage(mCheck.Name(), "ERROR")
	testLogger := logs.DiscardLogger()

	err := mCheck.RunCheck(testLogger)
	result := healthchecks.HealthCheckResult{Name: mCheck.Name(), Err: err}

	assert.ErrorContains(t, err, wantErrorMessage)
	assert.ErrorIs(t, err, healthchecks.HcFailureErr)
	assert.Check(t, strings.Contains(result.String(), expectedFailure))
	assert.Check(t, strings.Contains(result.String(), expectedError))

}

type MultipleSuccessResultCheck struct{}

func (c MultipleSuccessResultCheck) Name() string {
	return "MultipleResult Check"
}

func (c MultipleSuccessResultCheck) RunCheck(logger *logs.FileLogger) error {
	return errors.Join(nil, nil, nil)
}

func TestMultipleSuccessResultCheck(t *testing.T) {
	sCheck := MultipleSuccessResultCheck{}
	expectedSuccess := generateExpectedResultMessage(sCheck.Name(), "PASS")
	testLogger := logs.DiscardLogger()

	err := sCheck.RunCheck(testLogger)
	result := healthchecks.HealthCheckResult{Name: sCheck.Name(), Err: err}

	assert.NilError(t, err)
	assert.Check(t, strings.Contains(result.String(), expectedSuccess))
}
