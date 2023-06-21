# OTel exporter types

With the addition of the Prometheus and OTLP receivers, the OpenTelemetry
collector that Ops Agent runs can now handle multiple types of telemetry data.

## Collector pipelines

The collector has [three kinds of
pipeline](https://opentelemetry.io/docs/collector/configuration/#service),
`traces`, `metrics`, and `logs`. Each receiver component that can generate an
OTel pipeline must implement a method with the signature `Pipelines()
[]otel.ReceiverPipeline`. Each `ReceiverPipeline` object specifies a collector
receiver component (in `Receiver`) and a map (in `Processors`) from collector
pipeline type to a slice of processors for that pipeline type.

## OTel exporters

Ops Agent currently has three kinds of OTel exporters, which can be indicated by
setting the `ExporterTypes` field in a `ReceiverPipeline`.

### `OTel` (default)

The `OTel` exporter is a [`googlecloud`
exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/googlecloudexporter)
configured with `instrumentation_library_labels: true`. This means that metrics
are exported to `workload.googleapis.com/...` and have labels named
`instrumentation_source` and `instrumentation_version` from the OTel
[instrumentation
scope](https://opentelemetry.io/docs/specs/otel/glossary/#instrumentation-scope)
associated with each metric.

Built-in Ops Agent receiver types should use the `modifyscope` processor (via
`otel.ModifyInstrumentationScope`) to set an instrumentation scope of
`agent.googleapis.com/{receiver_type}`.

The `OTel` exporter handles trace data without any special configuration.

### `System`

The `System` exporter is a `googlecloud` exporter configured with
`instrumentation_library_labels: false`. It should only be used for pipelines
that generate metrics named `agent.googleapis.com/...` (i.e. system metrics).

### `GMP`

The `GMP` exporter is a [`googlemanagedprometheus`
exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/googlemanagedprometheusexporter)
with the default configuration. This means that any metrics will first be
transformed according to the [OTLP-to-Prometheus transformation
rules](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/#otlp-metric-points-to-prometheus)
and then written to the Cloud Monitoring API using the GMP transformation rules
(briefly, the metric names will be transformed to
`prometheus.googleapis.com/{metric_name}/{metric_kind}`, and resource attributes
will be transformed into a `prometheus_target` monitored resource.)

The `GMP` exporter does not support trace data.

Because the `GMP` exporter expects OTLP-format metrics, Ops Agent's Prometheus
receiver needs to first transform incoming Prometheus metrics into OTLP format,
using the inverse transformation (in the OTel document above). Then the metrics
are transformed back into Prometheus format by the exporter.
