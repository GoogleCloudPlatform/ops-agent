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
)

type HealthCheckFailure struct {
    // code string
    message string
    action string
    resourceLink string // TODO : Are strings the best for URLs ?
    isFatal bool
}

var healthCheckFailureMap = map[string]HealthCheckFailure{
    "port-unavailable" : {
        message: "Port is unavailable",
        action: "Check the port is available.",
        resourceLink : "",
        isFatal: true,
        // TODO : Add message with specific port
        // failMsg := fmt.Sprintf("listening to %s  was not successful.", net.JoinHostPort(host, port))
        // solMsg := fmt.Sprintf("verify the host %s is available to be used.", net.JoinHostPort(host, port))
    },
    "connection-to-logging-api-failed" : {
        message: "Request to Monitoring API failed.",
        action: "Check your internet connection.",
        resourceLink : "",
        isFatal: true,
    },
    "connection-to-monitoring-api-failed" : {
        message: "Request to Monitoring API failed.",
        action: "Check your internet connection.",
        resourceLink : "",
        isFatal: true,
    },
    "logging-api-missing-permission" : {
        message: "Service account misssing permissions for the Logging API.",
        action: "Add the logging.writer role to the GCP service account.",
        resourceLink : "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
        isFatal: true,
    },
    "monitoring-api-missing-permission" : {
        message: "Service account misssing permissions for the Monitoring API.",
        action: "Add the monitoring.writer role to the GCP service account.",
        resourceLink : "",
        isFatal: true,
    },
    "logging-api-disabled" : {
        // TODO : Add message with specific failure (e.g. Ping to api failed)
        // c.Fail("logging client didn't Ping successfully.", "check the logging api is enabled.")
        message: "The Logging API is disabled in the current GCP project.",
        action: "Check the Logging API is enabled",
        resourceLink : "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
        isFatal: true,
    },
    "monitoring-api-disabled" : {
        message: "The Monitoring API disabled",
        action: "Check the Monitoring API is disabled in the current GCP project.",
        resourceLink : "",
        isFatal: true,
    },
    "health-check-failure" : {
        message: "The Health Check failed.",
        action: "",
        resourceLink : "",
        isFatal: false,
    },
    "health-check-error" : {
        message: "The Health Check ran into an error.",
        action: "",
        resourceLink : "",
        isFatal: true,
        // TODO : Add message with specific error.
        // failMsg := fmt.Sprintf("listening to %s  was not successful.", net.JoinHostPort(host, port))
        // solMsg := fmt.Sprintf("verify the host %s is available to be used.", net.JoinHostPort(host, port))
    },
}

func GetFailure(failureCode string) (HealthCheckFailure, error) {
    if failure, ok := healthCheckFailureMap[failureCode]; ok {
        return failure, nil
    }

    return HealthCheckFailure{}, fmt.Errorf("The provided error code %s doesn't exist.", failureCode)
}