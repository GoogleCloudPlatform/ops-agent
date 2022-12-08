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
	message      string
	action       string
	resourceLink string // TODO : Are strings the best for URLs ?
	isFatal      bool
	Err          error
}

func (e HealthCheckError) Error() string {
	return e.Err.Error()
}

var (
	PORT_UNAVAILABLE_ERR = HealthCheckError{
		message:      "Port is unavailable",
		action:       "Check the port is available.",
		resourceLink: "",
		isFatal:      true,
		// TODO : Add message with specific port
		// failMsg := fmt.Sprintf("listening to %s  was not successful.", net.JoinHostPort(host, port))
		// solMsg := fmt.Sprintf("verify the host %s is available to be used.", net.JoinHostPort(host, port))
	}
	LOG_API_CONN_ERR = HealthCheckError{
		message:      "Request to Monitoring API failed.",
		action:       "Check your internet connection.",
		resourceLink: "",
		isFatal:      true,
	}
	MON_API_CONN_ERR = HealthCheckError{
		message:      "Request to Monitoring API failed.",
		action:       "Check your internet connection.",
		resourceLink: "",
		isFatal:      true,
	}
	LOG_API_PERMISSION_ERR = HealthCheckError{
		message:      "Service account misssing permissions for the Logging API.",
		action:       "Add the logging.writer role to the GCP service account.",
		resourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		isFatal:      true,
	}
	MON_API_PERMISSION_ERR = HealthCheckError{
		message:      "Service account misssing permissions for the Monitoring API.",
		action:       "Add the monitoring.writer role to the GCP service account.",
		resourceLink: "",
		isFatal:      true,
	}
	LOG_API_DISABLED_ERR = HealthCheckError{
		// TODO : Add message with specific failure (e.g. Ping to api failed)
		// c.Fail("logging client didn't Ping successfully.", "check the logging api is enabled.")
		message:      "The Logging API is disabled in the current GCP project.",
		action:       "Check the Logging API is enabled",
		resourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting#logging-module-logs",
		isFatal:      true,
	}
	MON_API_DISABLED_ERR = HealthCheckError{
		message:      "The Monitoring API disabled",
		action:       "Check the Monitoring API is disabled in the current GCP project.",
		resourceLink: "",
		isFatal:      true,
	}
	HC_FAILURE_ERR = HealthCheckError{
		message:      "The Health Check failed.",
		action:       "",
		resourceLink: "",
		isFatal:      false,
	}
)