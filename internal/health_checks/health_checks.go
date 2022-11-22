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
    RunCheck(uc *confgenerator.UnifiedConfig) (string, error)
    // GetUserFeedback() string
    // GetResult() string
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

func RunAllHealthChecks(uc *confgenerator.UnifiedConfig) error {

    for key, value := range GCEHealthChecks.healthCheckMap {
        fmt.Printf("%s %s\n", key, value)
        status, err := value.RunCheck(uc)
        fmt.Println(fmt.Sprintf("%s %s", status, err))
        /* if err !=  nil {
            return err
        } */
        // if err := NetworkCheck(); err != nil {
        //    fmt.Println(fmt.Sprintf("==> NetworkCheckErr : %s \n \n", err))
        //}      
    }
    return nil
}
