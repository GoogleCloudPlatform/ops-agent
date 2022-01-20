// Copyright 2021 Google LLC
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

package confgenerator

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// setLogNameComponents generates a series of components that rewrites the tag on log entries tagged `tag` to be `logName`.
func setLogNameComponents(tag, logName string) []fluentbit.Component {
	return []fluentbit.Component{
		{
			Kind: "FILTER",
			Config: map[string]string{
				"Match": tag,
				"Add":   fmt.Sprintf("logging.googleapis.com/logName %s", logName),
				"Name":  "modify",
			},
		},
	}
}

// stackdriverOutputComponent generates a component that outputs logs matching the regex `match` using `userAgent`.
func stackdriverOutputComponent(match, userAgent string) fluentbit.Component {
	return fluentbit.Component{
		Kind: "OUTPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
			"Name":              "stackdriver",
			"Match_Regex":       fmt.Sprintf("^(%s)$", match),
			"resource":          "gce_instance",
			"stackdriver_agent": userAgent,

			// https://docs.fluentbit.io/manual/administration/scheduling-and-retries
			// After 3 retries, a given chunk will be discarded. So bad entries don't accidentally stay around forever.
			"Retry_Limit": "3",

			// https://docs.fluentbit.io/manual/administration/security
			// Enable TLS support.
			"tls": "On",
			// Do not force certificate validation.
			"tls.verify": "Off",

			"workers": "8",

			// Mute these errors until https://github.com/fluent/fluent-bit/issues/4473 is fixed.
			"net.connect_timeout_log_error": "False",
		},
	}
}

// prometheusExporterOutputComponent generates a component that outputs to metrics prometheus format.
func prometheusExporterOutputComponent() fluentbit.Component {
	return fluentbit.Component{
		Kind: "OUTPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/outputs/prometheus-exporter
			"Name":  "prometheus_exporter",
			"Match": "*",
			"host":  "0.0.0.0",
			"port":  "20202",
		},
	}
}
