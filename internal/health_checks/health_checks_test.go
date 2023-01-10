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

package health_checks_test

import (
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/internal/health_checks"
	"gotest.tools/v3/assert"
)

type FailureCheck struct{}

func (c FailureCheck) Name() string {
	return "Failure Check"
}

func (c FailureCheck) RunCheck() error {
	return health_checks.HC_FAILURE_ERR
}

func TestCheckFailure(t *testing.T) {
	wantMessage := "The Health Check failed."
	wantAction := ""
	testCheck := FailureCheck{}

	err := testCheck.RunCheck()

	assert.ErrorType(t, err, health_checks.HealthCheckError{})
	healthError, _ := err.(health_checks.HealthCheckError)
	assert.Equal(t, wantMessage, healthError.Message)
	assert.Equal(t, wantAction, healthError.Action)
}

type SuccessCheck struct{}

func (c SuccessCheck) Name() string {
	return "Success Check"
}

func (c SuccessCheck) RunCheck() error {
	return nil
}

func TestCheckSuccess(t *testing.T) {
	testCheck := SuccessCheck{}

	err := testCheck.RunCheck()

	assert.NilError(t, err)
}

type ErrorCheck struct{}

func (c ErrorCheck) Name() string {
	return "Error Check"
}

func (c ErrorCheck) RunCheck() error {
	err := errors.New("Test error.")
	return err
}

func TestCheckError(t *testing.T) {
	wantMessage := "Test error."
	testCheck := ErrorCheck{}

	err := testCheck.RunCheck()

	assert.ErrorContains(t, err, wantMessage)
}

func TestRunAllHealthChecks(t *testing.T) {
	AllHealthChecks := health_checks.HealthCheckRegistry{
		FailureCheck{},
		SuccessCheck{},
		ErrorCheck{},
	}

	_, _ = AllHealthChecks.RunAllHealthChecks()
}
