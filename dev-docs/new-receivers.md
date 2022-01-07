# New 3rd Party Application Support How-To

## Steps to add new support

### 1. Research what telemetry the application generates

Figure out the following details for the application to facilitate the config design later.

- How many versions of the application are dominating in the industry, and whether we need to support more than one version.
- For each of the version we need to support, figure out:
  - Logging
    - What logs: Once installed, what type of logs the application writes by default (e.g. Apache access logs and Apache
      error logs). Any additional application settings users can change to enable additional logging.
    - Log file paths: The common log file paths for these logs.
    - Log formats: The log formats inside these files, and available customization options if users want to change the
      log format.
  - Metrics
    - What metrics: Once installed, what metrics the application exposes by default. Any additional application settings
      users can change to expose additional metrics.
    - How to expose metrics. If the application does not expose metrics by default (e.g. Apache does not), what
      application settings users need to specify to enable it.
    - Required user configurations (e.g. username) for the Ops Agent to talk to the application to get the metrics
      (e.g. username, password, database name, port, etc.)

Take Apache for example

NOTE: This is just an illustration, not the final design.

| What version(s) to support?                                      | Apache 2 seems to be dominating. We support that version only                                                                                                                 |
|------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| What logs?                                                       | Apache access log and Apache error log                                                                                                                                        |
| Additional logs the application can be configured to generate?   | None                                                                                                                                                                          |
| Default log file paths (RHEL / Red Hat / CentOS / Fedora)        | `/var/log/httpd/access_log and /var/log/httpd/error_log`                                                                                                                      |
| Default log file paths (Debian / Ubuntu)                         | `/var/log/apache2/access.log and /var/log/apache2/error.log`                                                                                                                  |
| Default log formats (Access log)                                 | `^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$` |
| Default log formats (Error log)                                  | `^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$`                                                 |
| Allowed user customization on logs                               | Arbitrary log paths and arbitrary format using a format string                                                                                                                |
| What metrics?                                                    | Not always on by default. Varies by installation.                                                                                                                             |
| What application settings users need to change to expose metrics | Requires the `mod_status` plugin to be enabled in Apache                                                                                                                        |
| Required user configuration for metrics scraping                 | URL                                                                                                                                                                           |

### 2. Design user visible interfaces

The user visible interfaces contain multiple components. The design could be done in parallel with some of the
implementation below. It’s important to frontload the alignment (e.g. via a doc) of these user visible interfaces to
avoid unnecessary delay at implementation or acceptance phases.

#### 2.1 Telemetry definitions - log name / labels, metrics namespace / name / type / labels

This determines when users query for the log entries / metrics data points, what they could filter by. This would later be mentioned in the public documentation.

#### 2.2 New logging / metrics receiver(s) type and allowed configuration parameters

This includes the receiver type, parameters (name, type, description, possible values), and a sample YAML config snippet
of that receiver. Overall we lean towards `User-centric view` instead of `Infrastructure-centric view` (definitions in
the `Configuration Asks Users Questions` section of https://sre.google/workbook/configuration-design/) when it comes to
Ops Agent config design. Below are some conventions we are trying to follow:

- Each application has its own receiver type(s).
  - Metrics side: Typically needs only one receiver type per application.
  - Logging side: Each log type maps to one receiver type (e.g. Apache has both access logs and error logs. So there
    will be two receiver types: `apache_access` and `apache_error`).
- 3rd party application log and metrics ingestion is not enabled in Ops Agent out of the box, so the built-in
  configuration does not contain 3rd party application specific configuration. Users need to define the pipeline(s) to
  enable log and metrics ingestion for an application (We will provide copy-pastable instructions).
- A `type` field is required for each receiver.
- The convention for receiver / processor ID is to use underscore as word delimiter. e.g. `apache_access`.
- Logging specific
  - The receiver, without customization, scrapes this application's logs from common log file paths and parses them with
    the common regex.
  - If the application allows users to customize the log file paths, the logging receiver(s) should have a corresponding
    `included_paths` parameter for custom log paths.
  - If the application allows users to customize the log formats, and we decide it’s commonly used, the logging
    receiver(s) should have a corresponding parameter (e.g. `format`) for custom log formats.
  - If we need to support more than one versions of the application, and the default log file paths / formats vary
    significantly across versions, the logging receivers should have a corresponding `version` parameter. And the
    default log file paths / formats might change based on the value of the `version` parameter.
- Metrics specific
  - The application specific metrics receiver(s) scrape the metrics data points from the application, and convert them into Google Cloud Monitoring data model formats.
  - A `collection_interval` parameter is highly recommended for each metrics receiver.
  - If there are required user settings (e.g. username, password) for the Ops Agent to talk to the application, the metrics receiver(s) should have corresponding parameter.
  - If we need to support more than one versions of the application, and the metrics setup vary significantly across versions, the metrics receivers should have a corresponding `version` parameter. And the way this receiver works might change based on the value of the `version` parameter.

Take Apache for example

Given the research result above, there will be 1 new metrics receiver (type: `apache`) and 2 logging receivers (type:
`apache_access` and `apache_error`) for Apache. A sample config snippet (detailed parameters are TBD) looks like:

```
logging:
  receivers:
    apache_access:
      type: apache_access
      included_paths: /var/log/my_access_log # Optional.
      format: # Optional
    apache_error:
      type: apache_error
      included_paths: /var/log/my_error_log # Optional.
      format: # Optional
  service:
    pipelines:
      apache:
        receivers: [apache_access, apache_error]
metrics:
  receivers:
    apache:
      type: apache
      endpoint: # Optional
      username: # Optional
      password: # Optional
  service:
    pipelines:
      apache:
        receivers: [apache]
```

#### 2.3 Validations against invalid configurations for the receiver(s)

Sample validation cases are like:
- A required parameter is not set for the given receiver(s)
- An unknown parameter is set for the receiver(s)
- A combination of parameters is not supported for the receiver(s). E.g. parameter A should be required when parameter B
  is present.
- The value of a certain parameter doesn't pass smell test (e.g. invalid url)

### 3. Complete any necessary upstream (Fluent Bit / OTEL) work

This is just a placeholder for any necessary upstream work at Fluent Bit and OTEL level if needed.

- Logging side (Fluent Bit): The generic Fluent Bit tail input plugin, regex / json parser plugins, and multiline plugin
  should be able to handle most application logs. In case any application has complex log format, some minor work might
  be needed.
- Metrics side: Some application metrics should be covered by generic OTEL receivers (e.g. Prometheus, JVM, curl JSON,
  HTTP) or might already have an application specific receiver that is supported by the community, while others require
  writing a new receiver. When it comes to metrics format transformation, typical applications can use the existing
  `metricstransform` Otel processor. Depending on how complex the transformation is, it might involving introducing new
  Otel processors as well.

This should include any integration tests needed at the upstream level.

### 4. Implement the Ops Agent level logging / metrics receiver(s)

This includes implementation for the new receiver(s) as designed in Step 2, the corresponding configuration validations,
and the config conversion from Ops Agent receiver(s) to the underlying subagent configurations:

- Metrics side: Requires conversion from the Ops Agent receiver(s) to the corresponding Otel receiver and processor configs.
- Logging side: Requires conversion from the Ops Agent receiver(s) to Fluent Bit tail input plugin configs and parser
  filter plugin configs (e.g. parse regex, multi line parsing etc. depending on the application)

Tip: https://regex101.com/ is your friend in case you need to debug Regex expressions.

This also includes [unit tests](https://github.com/GoogleCloudPlatform/ops-agent/tree/master/confgenerator/testdata) for
the config validation and conversion logic.

Follow the `Code Structure` section below for the implementation.

### 5. Add Ops Agent level smoke test for the application

Each application should at least have 1 smoke test that verifies the end-to-end experience involving:
- The Ops Agent successfully tails a log file or scraping metrics from an endpoint.
- The Ops Agent successfully ingests the logs or metrics via the Google Cloud Logging or Monitoring APIs.
- A query for the logs and metrics via API is successful.

Follow https://github.com/GoogleCloudPlatform/ops-agent/blob/master/integration_test/README.md for detailed instructions.

### 6. Release and Public documentation

To prepare for a formal release that includes support for this application, the corresponding public documentation needs
to be ready:

- Public documentation: https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent.
- Configuration specific page: https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/configuration.
- Each application has a specific page like: https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/nginx

For each application, the documentation needs to cover the following:
- How to enable logs / metrics ingestion for this application
  - If the application does not expose metrics by default, what users need to do to the application to enable it. This
    might be no-op for many applications.
  - How users customize the Ops Agent config file to enable scraping and ingesting telemetry for this application. This
    contains details about the new receiver type(s). For example:
    - The default log file paths the logging receiver(s) expect logs to be at
    - The default log formats the logging receiver(s) expect the logs to be in
    - The default metrics endpoint / port the metrics receiver expect metrics to be exposed at
    - Available parameters of the receiver(s) to control additional behaviors
  - Sample pipeline and receiver configuration snippets
- How to query logs and metrics for this application once ingested
  - When it comes to querying the logs and metrics data points, which log name, log labels, metrics namespace, metrics
    names, metrics labels to filter by.
  - Which dashboards contain these metrics by default.
- Common failure cases specific to this application and how to troubleshoot them.
  For each of the failure cases, document how users could detect that failure condition (e.g. via `nginx -V` to figure out whether the
  `stub status` module is included; via errors in the Ops Agent log, via Ops Agent’s own health metrics), and how to fix it.
  Sample failure conditions include:
  - Failed to talk to the application because the endpoint that exposes metrics is down.
  - Failed to connect to a database because the username and password combo is invalid

## Code structure

This describes the code structure to follow when adding new receivers to the Ops Agent repo.

Generic code is shared across applications:

- One set of code to load/parse a config file
- One set of code to merge a slice of config structs
- One set of code to validate a config struct
- One set of code to generate otel configs from a config struct
- One set of code to generate fluentbit configs from a config struct

And then one file per 3rd party application integration should contain the config struct(s) for that integration and any
integration-specific business logic. These files are defined inside https://github.com/GoogleCloudPlatform/ops-agent/tree/master/apps

- One file per application integration, defining receiver and processor struct(s) as needed for that application including both logging and metrics
- Validation is performed with struct tags on each config struct (~zero per-type validation code)
- Config structs have a method to generate OT and fluentbit config from the config struct. See details in the Pipelines() method below
- Each application registers its own receiver by specifying the receiver in the init() function of its own file. Adding support for a new application only needs to touch this one new file, unless it needs to add common utils.

Example code for IIS application

```go
package apps

import (
        "github.com/GoogleCloudPlatform/ops-agent/confgenerator"
        "github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
)

type MetricsReceiverIis struct {
	confgenerator.ConfigComponent `yaml:",inline"`

	// This is a convenience struct with common fields like CollectionInterval.
	confgenerator.MetricsReceiverShared `yaml:",inline"`
}

func (r MetricsReceiverIis) Type() string {
	// This is the string that will identify this receiver in user configs.
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
	confgenerator.MetricsReceiverTypes.RegisterType(func() confgenerator.Component { return &MetricsReceiverIis{} }, "windows")
}
```
