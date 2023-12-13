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
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
)

// TODO: The collector defaults to this, but should we default to 127.0.0.1 or ::1 instead?
const defaultGRPCEndpoint = "0.0.0.0:4317"

// Keep these in sync:
// https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/blob/main/exporter/collector/config.go#L158
var knownDomains = []string{"googleapis.com", "kubernetes.io", "istio.io", "knative.dev"}

type ReceiverOTLP struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	GRPCEndpoint string `yaml:"grpc_endpoint" validate:"omitempty,hostname_port" tracking:"endpoint"`
	MetricsMode  string `yaml:"metrics_mode" validate:"omitempty,oneof=googlecloudmonitoring googlemanagedprometheus" tracking:""`
}

func (r ReceiverOTLP) Type() string {
	return "otlp"
}

func (ReceiverOTLP) gmpResourceProcessors(ctx context.Context) []otel.Component {
	// Keep in sync with logic in confgenerator/prometheus.go
	stmt := func(target, source, platform string) string {
		// if cloud.provider == "gcp" && cloud.platform == "gcp_compute_engine"
		return fmt.Sprintf(`set(%s, %s) where %s != nil and resource.attributes["cloud.platform"] == "%s"`,
			target, source, source, platform)
	}
	resourceProcessorFunc := otel.GCPResourceDetector

	resource, autodetected, err := platform.FromContext(ctx).GetResource()
	if err != nil {
		log.Printf("can't get resource metadata: %v", err)
		return nil
	}
	if !autodetected {
		resourceProcessorFunc = func(override bool) otel.Component {
			return otel.ResourceTransform(resource.OTelResourceAttributes(), override)
		}
	}

	processor := resourceProcessorFunc(false)
	return []otel.Component{
		processor,
		{
			Type: "transform",
			Config: map[string]interface{}{
				"error_mode": "ignore",
				"metric_statements": []map[string]interface{}{{
					"context": "datapoint",
					"statements": []string{
						// GCE
						// location = cloud.availability_zone
						stmt(`attributes["location"]`, `resource.attributes["cloud.availability_zone"]`, "gcp_compute_engine"),
						// namespace = host.id
						stmt(`attributes["namespace"]`, `resource.attributes["host.id"]`, "gcp_compute_engine"),
						// cluster = "__gce__"
						stmt(`attributes["cluster"]`, `"__gce__"`, "gcp_compute_engine"),
						// instance_name = host.name
						stmt(`attributes["instance_name"]`, `resource.attributes["host.name"]`, "gcp_compute_engine"),
						// machine_type = host.type
						stmt(`attributes["machine_type"]`, `resource.attributes["host.type"]`, "gcp_compute_engine"),
					},
				}},
			},
		},
		// TODO: Can we just set resource.attributes instead of setting metric attributes and then grouping?
		otel.GroupByGMPAttrs(),
	}
}

func (r ReceiverOTLP) metricsProcessors(ctx context.Context) (otel.ExporterType, otel.ResourceDetectionMode, []otel.Component) {
	if r.MetricsMode != "googlecloudmonitoring" {
		return otel.GMP, otel.None, r.gmpResourceProcessors(ctx)
	}
	var knownDomainsRegexEscaped []string
	for _, knownDomain := range knownDomains {
		knownDomainsRegexEscaped = append(knownDomainsRegexEscaped, regexp.QuoteMeta(knownDomain))
	}
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
	return otel.OTel, otel.SetIfMissing, []otel.Component{
		otel.MetricsTransform(
			otel.RegexpRename(`^(.*)$`, `A${1}`),
			otel.RegexpRename(fmt.Sprintf(`^A((?:[a-z]+\.)*(?:%s)/.+)$`, strings.Join(knownDomainsRegexEscaped, "|")), `B${1}`),
			otel.RegexpRename(`^A(.*)$`, `Aworkload.googleapis.com/${1}`),
			otel.RegexpRename(`^[AB](.*)$`, `${1}`),
		),
		// N.B. We don't touch the instrumentation_scope fields here, so we can pass through the incoming strings.
	}
}

func (r ReceiverOTLP) Pipelines(ctx context.Context) []otel.ReceiverPipeline {
	endpoint := r.GRPCEndpoint
	if endpoint == "" {
		endpoint = defaultGRPCEndpoint
	}

	receiverPipelineType, metricsRDM, metricsProcessors := r.metricsProcessors(ctx)

	return []otel.ReceiverPipeline{{
		ExporterTypes: map[string]otel.ExporterType{
			"metrics": receiverPipelineType,
			"traces":  otel.OTel,
		},
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
			"metrics": metricsProcessors,
			"traces":  nil,
		},
		ResourceDetectionModes: map[string]otel.ResourceDetectionMode{
			"metrics": metricsRDM,
			"traces":  otel.SetIfMissing,
		},
	}}
}

func init() {
	confgenerator.CombinedReceiverTypes.RegisterType(func() confgenerator.CombinedReceiver { return &ReceiverOTLP{} })
}
