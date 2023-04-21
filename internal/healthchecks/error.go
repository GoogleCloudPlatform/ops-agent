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

// Error classification
const (
	Api        = "API"
	Connection = "CONNECTION"
	Generic    = "GENERIC"
	Port       = "PORT"
	Permission = "PERMISSION"
)

type HealthCheckError struct {
	Code         string
	Class        string
	Message      string
	Action       string
	ResourceLink string
	IsFatal      bool
}

func (e HealthCheckError) Error() string {
	return e.Message
}

// Interface used to verify if an error implements `Unwrap() []error`.
// The resulting error from `errors.Join(errs ...error)` implements this interface.
// This error features were added in Go 1.20 release (https://tip.golang.org/doc/go1.20).
type MultiWrappedError interface {
	Unwrap() []error
}

var (
	FbMetricsPortErr = HealthCheckError{
		Code:         "FbMetricsPortErr",
		Class:        Port,
		Message:      "Port 20202 needed for Ops Agent self metrics is unavailable.",
		Action:       "Verify that port 20202 is open.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	OtelMetricsPortErr = HealthCheckError{
		Code:         "OtelMetricsPortErr",
		Class:        Port,
		Message:      "Port 20201 needed for Ops Agent self metrics is unavailable.",
		Action:       "Verify that port 20201 is open.",
		ResourceLink: "https://cloud.google.com/monitoring/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	LogApiConnErr = HealthCheckError{
		Code:         "LogApiConnErr",
		Class:        Connection,
		Message:      "Request to Logging API failed.",
		Action:       "Check your internet connection and firewall rules.",
		ResourceLink: "https://cloud.google.com/logging/docs/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	MonApiConnErr = HealthCheckError{
		Code:         "MonApiConnErr",
		Class:        Connection,
		Message:      "Request to Monitoring API failed.",
		Action:       "Check your internet connection and firewall rules.",
		ResourceLink: "https://cloud.google.com/monitoring/agent/ops-agent/troubleshooting",
		IsFatal:      true,
	}
	PacApiConnErr = HealthCheckError{
		Code:         "PacApiConnErr",
		Class:        Connection,
		Message:      "Request to packages.cloud.google.com failed.",
		Action:       "Check your internet connection and firewall rules.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-run-ingest#network-issues",
		IsFatal:      false,
	}
	DLApiConnErr = HealthCheckError{
		Code:         "DLApiConnErr",
		Class:        Connection,
		Message:      "Request to dl.google.com failed",
		Action:       "Check your internet connection and firewall rules.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-run-ingest#network-issues",
		IsFatal:      false,
	}
	MetaApiConnErr = HealthCheckError{
		Code:         "MetaApiConnErr",
		Class:        Connection,
		Message:      "Request to GCE Metadata server failed",
		Action:       "Check your internet connection and firewall rules.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/troubleshoot-run-ingest#network-issues",
		IsFatal:      true,
	}
	LogApiScopeErr = HealthCheckError{
		Code:         "LogApiScopeErr",
		Class:        Permission,
		Message:      "VM is missing the https://www.googleapis.com/auth/logging.write scope.",
		Action:       "Add the https://www.googleapis.com/auth/logging.write scope to the Compute Engine VM.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization",
		IsFatal:      true,
	}
	MonApiScopeErr = HealthCheckError{
		Code:         "MonApiScopeErr",
		Class:        Permission,
		Message:      "VM is missing the https://www.googleapis.com/auth/monitoring.write scope.",
		Action:       "Add the https://www.googleapis.com/auth/monitoring.write scope to the Compute Engine VM.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization",
		IsFatal:      true,
	}
	LogApiPermissionErr = HealthCheckError{
		Code:         "LogApiPermissionErr",
		Class:        Permission,
		Message:      "Service account is missing the roles/logging.logWriter role.",
		Action:       "Add the roles/logging.logWriter role to the Google Cloud service account.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization#create-service-account",
		IsFatal:      true,
	}
	MonApiPermissionErr = HealthCheckError{
		Code:         "MonApiPermissionErr",
		Class:        Permission,
		Message:      "Service account is missing the roles/monitoring.metricWriter role.",
		Action:       "Add the roles/monitoring.metricWriter role to the Google Cloud service account.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization#create-service-account",
		IsFatal:      true,
	}
	LogApiDisabledErr = HealthCheckError{
		Code:         "LogApiDisabledErr",
		Class:        Api,
		Message:      "The Logging API is disabled in the current Google Cloud project.",
		Action:       "Enable Logging API in the current Google Cloud project.",
		ResourceLink: "https://cloud.google.com/logging/docs/api/enable-api",
		IsFatal:      true,
	}
	MonApiDisabledErr = HealthCheckError{
		Code:         "MonApiDisabledErr",
		Class:        Api,
		Message:      "The Monitoring API is disabled in the current Google Cloud project.",
		Action:       "Enable Monitoring API in the current Google Cloud project.",
		ResourceLink: "https://cloud.google.com/monitoring/api/enable-api",
		IsFatal:      true,
	}
	LogApiUnauthenticatedErr = HealthCheckError{
		Code:         "LogApiUnauthenticatedErr",
		Class:        Api,
		Message:      "The current VM couldn't authenticate to the Logging API.",
		Action:       "Verify that your credential files, scopes and permissions are set up correctly.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization",
		IsFatal:      true,
	}
	MonApiUnauthenticatedErr = HealthCheckError{
		Code:         "MonApiUnauthenticatedErr",
		Class:        Api,
		Message:      "The current VM couldn't authenticate to the Monitoring API.",
		Action:       "Verify that your credential files, scopes and permissions are set up correctly.",
		ResourceLink: "https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/authorization",
		IsFatal:      true,
	}
	HcFailureErr = HealthCheckError{
		Code:         "HcFailureErr",
		Class:        Generic,
		Message:      "The Health Check encountered an internal error.",
		Action:       "Submit a support case from Google Cloud console.",
		ResourceLink: "https://cloud.google.com/logging/docs/support",
		IsFatal:      false,
	}
)
