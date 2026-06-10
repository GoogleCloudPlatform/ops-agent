# Agent Metrics Processor

Supported pipeline types: metrics

This is a **temporary** processor for supporting conversions from system metrics
that follow the OpenTelemetry conventions to "agent" metrics in the format
expected by Cloud Monitoring, that aren't currently supported by the Metric
Transform processor:

1. Translate from metrics that store process information as resources to metrics
   that store process information as labels (this will be moved into the Cloud
   Ops exporter once migration to new pipeline is completed).
2. Split metrics with read/write direction labels into two separate metrics (the
   metrics transform processor should be extended to support this functionality
   out of the box).
3. Creates utilization metrics out of usage metrics by dividing the values of
   each data point by the sum across a particular label / dimension (the
   metrics transform processor should be extended to support this functionality
   out of the box).

## Configuration

No additional configuration is currently possible. This processor is only expected
to be used in a pipeline that includes the Host Metrics receiver and Google Cloud
exporter and should generally be the first processor in the pipeline, i.e.

```yaml
service:
  pipelines:
    metrics:
      receivers: [hostmetrics]
      processors: [agentmetrics, ...]
      exporters: [googlecloud]
```
