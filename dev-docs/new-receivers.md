# Receiver How-To

This describes the code structure to follow when adding new receivers to the Ops Agent repo.

Generic code is shared across applications:

- One set of code to load/parse a config file
- One set of code to merge a slice of config structs
- One set of code to validate a config struct
- One set of code to generate otel configs from a config struct
- One set of code to generate fluentbit configs from a config struct

And then one file per 3rd party application integration should contain the config struct(s) for that integration and any integration-specific business logic. These files are defined inside https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator

- One file per application integration, defining receiver and processor struct(s) as needed for that application including both logging and metrics
- Validation is performed with struct tags on each config struct (~zero per-type validation code)
- Config structs have a method to generate OT and fluentbit config from the config struct. See details in the Pipelines() method below
- Each application register its own receiver by specifying the receiver in the init() function of its own file. Adding support for a new application only needs to touch this one new file, unless it needs to add common utils. 

Related PRs:

- https://github.com/GoogleCloudPlatform/ops-agent/pull/143 
- https://github.com/GoogleCloudPlatform/ops-agent/pull/145 
- https://github.com/GoogleCloudPlatform/ops-agent/pull/146 

## Example code for IIS application

```go
package confgenerator
 
import "github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
 
type MetricsReceiverIis struct {
	ConfigComponent `yaml:",inline"`
 
	MetricsReceiverShared `yaml:",inline"`
}
 
func (r MetricsReceiverIis) Type() string {
	return "iis"
}
 
func (r MetricsReceiverIis) Pipelines() []otel.Pipeline {
	return []otel.Pipeline{{
		Receiver: otel.Component{
			Type: "windowsperfcounters",
			Config: map[string]interface{}{
				"collection_interval": r.CollectionIntervalString(),
				"perfcounters": []map[string]interface{}{
					{
						"object":    "Web Service",
						"instances": []string{"_Total"},
						"counters": []string{
							"Current Connections",
							"Total Bytes Received",
							"Total Bytes Sent",
							"Total Connection Attempts (all instances)",
							"Total Delete Requests",
							"Total Get Requests",
							"Total Head Requests",
							"Total Options Requests",
							"Total Post Requests",
							"Total Put Requests",
							"Total Trace Requests",
						},
					},
				},
			},
		},
		Processors: []otel.Component{
			otel.MetricsTransform(
				otel.RenameMetric(
					`\Web Service(_Total)\Current Connections`,
					"iis/current_connections",
				),
				otel.CombineMetrics(
					`^\\Web Service\(_Total\)\\Total Bytes (?P<direction>.*)$`,
					"iis/network/transferred_bytes_count",
				),
				otel.RenameMetric(
					`\Web Service(_Total)\Total Connection Attempts (all instances)`,
					"iis/new_connection_count",
				),
				otel.CombineMetrics(
					`^\\Web Service\(_Total\)\\Total (?P<http_method>.*) Requests$`,
					"iis/request_count",
				),
				otel.AddPrefix("agent.googleapis.com"),
			),
		},
	}}
}
 
func init() {
	metricsReceiverTypes.registerType(func() component { return &MetricsReceiverIis{} }, "windows")
}
```

