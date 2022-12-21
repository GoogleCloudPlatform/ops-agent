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

package apps

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

// TODO: The collector defaults to this, but should we default to 127.0.0.1 or ::1 instead?
const defaultGRPCEndpoint = "0.0.0.0:4317"

// Keep these in sync:
// https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/blob/main/exporter/collector/config.go#L158
var knownDomains = []string{"googleapis.com", "kubernetes.io", "istio.io", "knative.dev"}

type ReceiverOTLP struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	GRPCEndpoint string `yaml:"grpc_endpoint" validate:"omitempty,hostname_port"`
}

func (r ReceiverOTLP) Type() string {
	return "otlp"
}

func (r ReceiverOTLP) Pipelines() []otel.ReceiverPipeline {
	endpoint := r.GRPCEndpoint
	if endpoint == "" {
		endpoint = defaultGRPCEndpoint
	}
	var knownDomainsRegexEscaped []string
	for _, knownDomain := range knownDomains {
		knownDomainsRegexEscaped = append(knownDomainsRegexEscaped, regexp.QuoteMeta(knownDomain))
	}
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type: "otlp",
			Config: map[string]interface{}{
				"protocols": map[string]interface{}{
					"grpc": map[string]interface{}{
						"endpoint": endpoint,
					},
				},
			},
		},
		Processors: map[string][]otel.Component{
			// The intent here is to add the workload.googleapis.com prefix to any metrics
			// that do not match the list of known domains in the exporter [1].
			//
			// This would ordinarily be accomplished using a metricstransform processor
			// with a negative-lookahead regexp, but Go regexp (the regexp engine used by
			// metricstransform) does not support such a thing. Emulating a negative-lookahead
			// pattern in Go regexp might be possible but I tried it and it melted my brain.
			//
			// The metricstransform processor does not support negative matching either.
			//
			// So instead we apply a sequence of transformations:
			// 1) Prefix all metrics with 'A'.
			// 2) Replace 'A' with 'B' if the metric name matches a known domain.
			// 3) All metrics that still have 'A' are the ones that don't match any known domain.
			//    For these, replace 'A' with 'Aworkload.googleapis.com/' (we keep the 'A').
			// 4) All metrics have either 'A' or 'B' at the start; remove it.
			//    At this point, we have prefixed all metrics with 'workload.googleapis.com/'
			//    if they did not match any known domains.
			//
			// TODO: get OTEL to split the prefixing behaviour of the googlecloud exporter out as
			// a processor and replace all of this stuff with that processor.
			//
			// [1] https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/blob/main/exporter/collector/config.go#L158
			"metrics": {
				otel.MetricsTransform(
					otel.RegexpRename(`^(.*)$`, `A${1}`),
					otel.RegexpRename(fmt.Sprintf(`^A((?:[a-z]+\.)*(?:%s)/.+)$`, strings.Join(knownDomainsRegexEscaped, "|")), `B${1}`),
					otel.RegexpRename(`^A(.*)$`, `Aworkload.googleapis.com/${1}`),
					otel.RegexpRename(`^[AB](.*)$`, `${1}`),
				),
				// TODO: Set instrumentation_source labels, etc.
			},
			"traces": nil,
		},
	}}
}

func init() {
	confgenerator.CombinedReceiverTypes.RegisterType(func() confgenerator.CombinedReceiver { return &ReceiverOTLP{} })
}
