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
	"testing"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/internal/health_checks"
	"gotest.tools/v3/assert"
)

type FailureCheck struct {
	health_checks.HealthCheck
}

func (c FailureCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
	c.Fail("Test failure.", "Test message.")
	return nil
}

func TestCheckFailure(t *testing.T) {
	wantResult := "FAIL"
	wantFailure := "Test failure."
	wantSolution := "Test message."

	emptyConfig := &confgenerator.UnifiedConfig{}
	testCheck := &FailureCheck{HealthCheck: health_checks.NewHealthCheck()}

	err := testCheck.RunCheck(emptyConfig)
	assert.NilError(t, err)
	assert.Equal(t, wantResult, testCheck.GetResult())
	assert.Equal(t, wantFailure, testCheck.GetFailureMessage())
	assert.Equal(t, wantSolution, testCheck.GetSolutionMessage())
}

type SuccessCheck struct {
	health_checks.HealthCheck
}

func (c SuccessCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
	return nil
}

func TestCheckSuccess(t *testing.T) {
	wantResult := "PASS"
	wantFailure := ""
	wantSolution := ""
	emptyConfig := &confgenerator.UnifiedConfig{}
	testCheck := &SuccessCheck{HealthCheck: health_checks.NewHealthCheck()}

	err := testCheck.RunCheck(emptyConfig)
	assert.NilError(t, err)
	assert.Equal(t, wantResult, testCheck.GetResult())
	assert.Equal(t, wantFailure, testCheck.GetFailureMessage())
	assert.Equal(t, wantSolution, testCheck.GetSolutionMessage())
}
