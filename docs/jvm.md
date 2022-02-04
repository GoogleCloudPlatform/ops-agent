# JVM

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/jvm)
for instructions to collect metrics from this application using Ops Agent.

## Metrics

The following table provides the list of metrics that the Ops Agent collects from this application.

| Metric                                             | Data Type | Unit        | Labels | Description |
| ---                                                | ---       | ---         | ---    | ---         | 
| workload.googleapis.com/jvm.classes.loaded         | gauge     | 1           |        | Current number of loaded classes |
| workload.googleapis.com/jvm.gc.collections.count   | cumulative       | 1           | name   | Total number of garbage collections |
| workload.googleapis.com/jvm.gc.collections.elapsed | cumulative       | ms          | name   | Time spent garbage collecting |
| workload.googleapis.com/jvm.memory.heap            | gauge     | by          |        | Current heap usage |
| workload.googleapis.com/jvm.memory.nonheap         | gauge     | by          |        | Current non-heap usage |
| workload.googleapis.com/jvm.memory.pool            | gauge     | by          | name   | Current memory pool usage |
| workload.googleapis.com/jvm.threads.count          | gauge     | 1           |        | Current number of threads |
