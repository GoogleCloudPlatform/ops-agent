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
    "go.uber.org/multierr"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator/resourcedetector"
    "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
)

var (
    // MetadataResource is the resource metadata for the instance we're running on.
    // Note: This is a global variable so that it can be set in tests.
    MetadataResource resourcedetector.Resource

    config *confgenerator.UnifiedConfig
)

type HealthCheck interface {
    RunCheck(uc *confgenerator.UnifiedConfig) error
    Fail(failMsg string, solMsg string)
    GetResult() string
    LogMessage(message string)
    GetLogMessage() string
    GetFailureMessage() string
    GetSolutionMessage() string
}

type healthCheckRegistry struct {
    environment string
    healthCheckMap map[string]HealthCheck
}

func (r healthCheckRegistry) RegisterCheck(name string, c HealthCheck) {
    r.healthCheckMap[name] = c
}

var GCEHealthChecks = &healthCheckRegistry{
    environment: "GCE",
    healthCheckMap: make(map[string]HealthCheck),
}

type BaseHealthCheck struct {
    HealthCheck
    failed bool
    logMessage string
    failureMessage string
    solutionMessage string
}

func NewHealthCheck() HealthCheck {
    return &BaseHealthCheck{
        failed: false,
        logMessage: "",
        failureMessage: "",
        solutionMessage: "",
    }
}

func (b *BaseHealthCheck) RunCheck(uc *confgenerator.UnifiedConfig) error {
    return nil
}

func (b *BaseHealthCheck) Fail(failMsg string, solMsg string) {
    b.failed = true
    b.failureMessage = failMsg
    b.solutionMessage = solMsg
}

func (b *BaseHealthCheck) LogMessage(message string) {
    b.logMessage = b.logMessage + "\n" + message
}

func (b *BaseHealthCheck) GetLogMessage() string {
    return b.logMessage
}

func (b *BaseHealthCheck) GetFailureMessage() string {
    return b.failureMessage
}

func (b *BaseHealthCheck) GetSolutionMessage() string {
    return b.solutionMessage
}

func (b *BaseHealthCheck) GetResult() string {
    if b.failed {
        return "FAIL"
    } else {
        return "PASS"
    } 
}

func RunAllHealthChecks(uc *confgenerator.UnifiedConfig) error {
    var multiErr error
    fmt.Println("========================================")
    fmt.Println("Health Checks : \n")
    for name, c := range GCEHealthChecks.healthCheckMap {

        err := c.RunCheck(uc)
        if err !=  nil {
            fmt.Println(fmt.Sprintf("%s", err))
            multierr.Append(multiErr, err)
        }    

        fmt.Printf("Check: %s, Status: %s \n", name, c.GetResult())
        fmt.Printf("Failure: %s \n", c.GetFailureMessage())
        fmt.Printf("Solution: %s \n\n", c.GetSolutionMessage())
        // fmt.Println("Log : " + c.GetLogMessage())
    }
    fmt.Println("========================================")

    return multiErr
}
