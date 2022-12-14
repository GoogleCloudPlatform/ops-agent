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

type HealthCheckError struct {
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
	PORT_UNAVAILABLE_ERR = HealthCheckError{
		Message:      "Port is unavailable",
		Action:       "Check the port is available.",
		ResourceLink: "",
		IsFatal:      true,
		// TODO : Add Message with specific port
		// failMsg := fmt.Sprintf("listening to %s  was not successful.", net.JoinHostPort(host, port))
		// solMsg := fmt.Sprintf("verify the host %s is available to be used.", net.JoinHostPort(host, port))
	}
	LOG_API_CONN_ERR = HealthCheckError{
		Message:      "Request to Monitoring API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "",
		IsFatal:      true,
	}
	MON_API_CONN_ERR = HealthCheckError{
		Message:      "Request to Monitoring API failed.",
		Action:       "Check your internet connection.",
		ResourceLink: "",
		IsFatal:      true,
	}
	LOG_API_PERMISSION_ERR = HealthCheckError{
		Message:      "Service account misssing permissions for the Logging API.",
		Action:       "Add the logging.writer role to the GCP service account.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MON_API_PERMISSION_ERR = HealthCheckError{
		Message:      "Service account misssing permissions for the Monitoring API.",
		Action:       "Add the monitoring.writer role to the GCP service account.",
		ResourceLink: "",
		IsFatal:      true,
	}
	LOG_API_DISABLED_ERR = HealthCheckError{
		// TODO : Add Message with specific failure (e.g. Ping to api failed)
		// c.Fail("logging client didn't Ping successfully.", "check the logging api is enabled.")
		Message:      "The Logging API is disabled in the current GCP project.",
		Action:       "Check the Logging API is enabled",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		IsFatal:      true,
	}
	MON_API_DISABLED_ERR = HealthCheckError{
		Message:      "The Monitoring API disabled",
		Action:       "Check the Monitoring API is disabled in the current GCP project.",
		ResourceLink: "",
		IsFatal:      true,
	}
	HC_FAILURE_ERR = HealthCheckError{
		Message:      "The Health Check failed.",
		Action:       "",
		ResourceLink: "",
		IsFatal:      false,
	}
)