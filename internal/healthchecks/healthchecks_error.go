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

type HealthCheckError struct {
	Code         string
	Class        string
	Message      string
	Action       string
	ResourceLink string
	IsFatal      bool
	Err          error
}

func (e HealthCheckError) Error() string {
	return e.Err.Error()
}

var (
	FbMetricsPortErr = HealthCheckError{
		Code:         "FbMetricsPortErr",
		Class:        "PORT",
		Message:      "Port 20202 used for fluent-bit self metrics is unavailable.",
		Action:       "Verify the 20202 port is available.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	OtelMetricsPortErr = HealthCheckError{
		Code:         "OtelMetricsPortErr",
		Class:        "PORT",
		Message:      "Port 20201 used for open telemetry self metrics is unavailable.",
		Action:       "Verify the 20201 port is available.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LogApiConnErr = HealthCheckError{
		Code:         "LogApiConnErr",
		Class:        "CONNECTION",
		Message:      "Request to Logging API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	MonApiConnErr = HealthCheckError{
		Code:         "MonApiConnErr",
		Class:        "CONNECTION",
		Message:      "Request to Monitoring API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LogApiScopeErr = HealthCheckError{
		Code:         "LogApiScopeErr",
		Class:        "PERMISSION",
		Message:      "VM has not enough access scopes for the Logging API.",
		Action:       "Add the https://www.googleapis.com/auth/logging.write scope to the GCP VM.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MonApiScopeErr = HealthCheckError{
		Code:         "MonApiScopeErr",
		Class:        "PERMISSION",
		Message:      "VM has not enough access scopes for the Monitoring API.",
		Action:       "Add the https://www.googleapis.com/auth/monitoring.write scope to the GCP VM.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LogApiPermissionErr = HealthCheckError{
		Code:         "LogApiPermissionErr",
		Class:        "PERMISSION",
		Message:      "Service account misssing permissions for the Logging API.",
		Action:       "Add the logging.writer role to the GCP service account.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MonApiPermissionErr = HealthCheckError{
		Code:         "MonApiPermissionErr",
		Class:        "PERMISSION",
		Message:      "Service account misssing permissions for the Monitoring API.",
		Action:       "Add the monitoring.writer role to the GCP service account.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LogApiDisabledErr = HealthCheckError{
		Code:         "LogApiDisabledErr",
		Class:        "API",
		Message:      "The Logging API is disabled in the current GCP project.",
		Action:       "Enable Logging API in the current GCP project.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MonApiDisabledErr = HealthCheckError{
		Code:         "MonApiDisabledErr",
		Class:        "API",
		Message:      "The Monitoring API is disabled in the current GCP project.",
		Action:       "Enable Monitoring API in the current GCP project.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	HcFailureErr = HealthCheckError{
		Code:         "HcFailureErr",
		Class:        "GENERIC",
		Message:      "The Health Check failed.",
		Action:       "No suggested action.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      false,
	}
)
