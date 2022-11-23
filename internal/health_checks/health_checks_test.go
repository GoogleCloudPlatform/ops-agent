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
	"regexp"
	"github.com/GoogleCloudPlatform/ops-agent/internal/health_checks"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)


type TestCheck struct{
    health_checks.HealthCheck
}

func (c TestCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
    c.Fail("Test failure.", "Test message")
    return nil
}

func TestCheckFailure(t *testing.T) {
    want := regexp.MustCompile(`\bFAIL\b`)
    emptyConfig := &confgenerator.UnifiedConfig{}
    testCheck := &TestCheck{HealthCheck: health_checks.NewHealthCheck()}

    err := testCheck.RunCheck(emptyConfig)
    if !want.MatchString(testCheck.GetResult()) || err != nil {
        t.Fatalf(`RunCheck() = %q, %v, want match for %#q, nil`, testCheck.GetResult(), err, want)
    }
}