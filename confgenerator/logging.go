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
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

const InstrumentationSourceLabel = `labels."logging.googleapis.com/instrumentation_source"`
const HttpRequestKey = "logging.googleapis.com/httpRequest"

func setLogNameProcessor(ctx context.Context, logName string) LoggingProcessorModifyFields {
	p := platform.FromContext(ctx)
	hostName := p.Hostname()
	fields := map[string]*ModifyField{
		"logName": {
			DefaultValue: &logName,
		},
		`labels."compute.googleapis.com/resource_name"`: {
			DefaultValue: &hostName,
		},
	}
	r, err := p.GetResource()
	if err == nil {
		for k, v := range r.ExtraLogLabels() {
			v := v
			fields[fmt.Sprintf("labels.%q", k)] = &ModifyField{
				DefaultValue: &v,
			}
		}
	}
	return LoggingProcessorModifyFields{
		Fields: fields,
	}
}

// setLogNameComponents generates a series of components that rewrites the tag on log entries tagged `tag` to be `logName`.
func setLogNameComponents(ctx context.Context, tag, logName, receiverType string) []fluentbit.Component {
	p := setLogNameProcessor(ctx, logName)
	p.Fields[`labels."agent.googleapis.com/log_file_path"`] = &ModifyField{
		MoveFrom: `jsonPayload."agent.googleapis.com/log_file_path"`,
	}
	// p.Fields[`labels."agent.googleapis.com/receiver_type"`] = &ModifyField{
	// 	StaticValue: &receiverType,
	// }
	return p.Components(ctx, tag, "setlogname")
}

func otelSetLogNameComponents(ctx context.Context, logName string) []otel.Component {
	// TODO: Prepend `receiver_id.` if it already exists, like the `fluent_forward` receiver?
	p := setLogNameProcessor(ctx, logName)
	components, err := p.Processors(ctx)
	if err != nil {
		// We're generating a hard-coded config, so this should never fail.
		panic(err)
	}
	return components
}

// stackdriverOutputComponent generates a component that outputs logs matching the regex `match` using `userAgent`.
func stackdriverOutputComponent(ctx context.Context, match, userAgent, storageLimitSize, compress string) fluentbit.Component {
	config := map[string]string{
		// https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
		"Name":              "stackdriver",
		"Match_Regex":       fmt.Sprintf("^(%s)$", match),
		"resource":          "gce_instance",
		"stackdriver_agent": userAgent,

		"http_request_key": HttpRequestKey,

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
	}
	if r := platform.FromContext(ctx).ResourceOverride; r != nil {
		mr := r.MonitoredResource()
		var labels []string
		for k, v := range mr.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(labels)
		config["resource"] = mr.Type
		config["resource_labels"] = strings.Join(labels, ",")
	}

	if storageLimitSize != "" {
		// Limit the maximum number of fluent-bit chunks in the filesystem for the current
		// output logical destination.
		config["storage.total_limit_size"] = storageLimitSize
	}

	if compress != "" {
		// Add payload compression
		config["compress"] = compress
	}

	return fluentbit.Component{
		Kind:   "OUTPUT",
		Config: config,
	}
}
