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
	FB_METRICS_PORT_ERR = HealthCheckError{
		Code:         "FB_METRICS_PORT_ERR",
		Class:        "PORT",
		Message:      "Port 20202 used for fluent-bit self metrics is unavailable.",
		Action:       "Verify the 20202 port is available.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	OTEL_METRICS_PORT_ERR = HealthCheckError{
		Code:         "OTEL_METRICS_PORT_ERR",
		Class:        "PORT",
		Message:      "Port 20201 used for open telemetry self metrics is unavailable.",
		Action:       "Verify the 20201 port is available.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LOG_API_CONN_ERR = HealthCheckError{
		Code:         "LOG_API_CONN_ERR",
		Class:        "CONNECTION",
		Message:      "Request to Logging API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	MON_API_CONN_ERR = HealthCheckError{
		Code:         "MON_API_CONN_ERR",
		Class:        "CONNECTION",
		Message:      "Request to Monitoring API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LOG_API_SCOPE_ERR = HealthCheckError{
		Code:         "LOG_API_SCOPE_ERR",
		Class:        "PERMISSION",
		Message:      "VM has not enough access scopes for the Logging API.",
		Action:       "Add the https://www.googleapis.com/auth/logging.write scope to the GCP VM.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MON_API_SCOPE_ERR = HealthCheckError{
		Code:         "MON_API_SCOPE_ERR",
		Class:        "PERMISSION",
		Message:      "VM has not enough access scopes for the Monitoring API.",
		Action:       "Add the https://www.googleapis.com/auth/monitoring.write scope to the GCP VM.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LOG_API_PERMISSION_ERR = HealthCheckError{
		Code:         "LOG_API_PERMISSION_ERR",
		Class:        "PERMISSION",
		Message:      "Service account misssing permissions for the Logging API.",
		Action:       "Add the logging.writer role to the GCP service account.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MON_API_PERMISSION_ERR = HealthCheckError{
		Code:         "MON_API_PERMISSION_ERR",
		Class:        "PERMISSION",
		Message:      "Service account misssing permissions for the Monitoring API.",
		Action:       "Add the monitoring.writer role to the GCP service account.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LOG_API_DISABLED_ERR = HealthCheckError{
		Code:         "LOG_API_DISABLED_ERR",
		Class:        "API",
		Message:      "The Logging API is disabled in the current GCP project.",
		Action:       "Enable Logging API in the current GCP project.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MON_API_DISABLED_ERR = HealthCheckError{
		Code:         "MON_API_DISABLED_ERR",
		Class:        "API",
		Message:      "The Monitoring API is disabled in the current GCP project.",
		Action:       "Enable Monitoring API in the current GCP project.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	HC_FAILURE_ERR = HealthCheckError{
		Code:         "HC_FAILURE_ERR",
		Class:        "GENERIC",
		Message:      "The Health Check failed.",
		Action:       "No suggested action.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      false,
	}
)
