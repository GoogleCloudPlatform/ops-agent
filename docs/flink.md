# Apache Flink

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/flink)
for instructions to collect logs and metrics from this application using Ops Agent.

## Metrics

The Ops Agent collects the following metrics from your flink instance.

| Metric                                | Data Type  | Unit            | Labels                 | Resource Types          | Description                                                                                                   |
|---------------------------------------|------------|-----------------|------------------------|-------------------------|---------------------------------------------------------------------------------------------------------------|
| flink.jvm.cpu.load                    | gauge      | "%"             |                        | jobmanager, taskmanager | The CPU usage of the JVM for a jobmanager or taskmanager.                                                     |
| flink.jvm.cpu.time                    | cumulative | ns              |                        | jobmanager, taskmanager | The CPU time used by the JVM for a jobmanager or taskmanager.                                                 |
| flink.jvm.memory.heap                 | cumulative | By              |                        | jobmanager, taskmanager | The amount of heap memory currently used.                                                                     |
| flink.jvm.memory.heap                 | cumulative | By              |                        | jobmanager, taskmanager | The amount of heap memory guaranteed to be available to the JVM.                                              |
| flink.jvm.memory.heap                 | cumulative | By              |                        | jobmanager, taskmanager | The maximum amount of heap memory that can be used for memory management.                                     |
| flink.jvm.memory.nonheap              | cumulative | By              |                        | jobmanager, taskmanager | The amount of non-heap memory currently used.                                                                 |
| flink.jvm.memory.nonheap              | cumulative | By              |                        | jobmanager, taskmanager | The amount of non-heap memory guaranteed to be available to the JVM.                                          |
| flink.jvm.memory.nonheap              | cumulative | By              |                        | jobmanager, taskmanager | The maximum amount of non-heap memory that can be used for memory management.                                 |
| flink.jvm.memory.metaspace            | cumulative | By              |                        | jobmanager, taskmanager | The amount of memory currently used in the Metaspace memory pool.                                             |
| flink.jvm.memory.metaspace            | cumulative | By              |                        | jobmanager, taskmanager | The amount of memory guaranteed to be available to the JVM in the Metaspace memory pool.                      |
| flink.jvm.memory.metaspace            | cumulative | By              |                        | jobmanager, taskmanager | The maximum amount of memory that can be used in the Metaspace memory pool.                                   |
| flink.jvm.memory.direct               | cumulative | By              |                        | jobmanager, taskmanager | The amount of memory used by the JVM for the direct buffer pool.                                              |
| flink.jvm.memory.direct               | cumulative | By              |                        | jobmanager, taskmanager | The total capacity of all buffers in the direct buffer pool.                                                  |
| flink.jvm.memory.mapped               | cumulative | By              |                        | jobmanager, taskmanager | The amount of memory used by the JVM for the mapped buffer pool.                                              |
| flink.jvm.memory.mapped               | cumulative | By              |                        | jobmanager, taskmanager | The number of buffers in the mapped buffer pool.                                                              |
| flink.memory.managed.used             | cumulative | By              |                        | jobmanager, taskmanager | The amount of managed memory currently used.                                                                  |
| flink.memory.managed.total            | cumulative | By              |                        | jobmanager, taskmanager | The total amount of managed memory.                                                                           |
| flink.jvm.threads.count               | cumulative | "{threads}"     |                        | jobmanager, taskmanager | The total number of live threads.                                                                             |
| flink.jvm.gc.collections              | cumulative | "{collections}" | garbage_collector_name | jobmanager, taskmanager | The total number of collections that have occurred.                                                           |
| flink.jvm.gc.collections              | cumulative | ms              | garbage_collector_name | jobmanager, taskmanager | The total time spent performing garbage collection.                                                           |
| flink.jvm.class_loader.classes_loaded | cumulative | "{classes}"     |                        | jobmanager, taskmanager | The total number of classes loaded since the start of the JVM.                                                |
| flink.job.restart.count               | cumulative | "{restarts}"    |                        | job                     | The total number of restarts since this job was submitted, including full restarts and fine-grained restarts. |
| flink.job.last_checkpoint.time        | gauge      | ms              |                        | job                     | The end to end duration of the last checkpoint.                                                               |
| flink.job.last_checkpoint.size        | cumulative | By              |                        | job                     | The total size of the last checkpoint.                                                                        |
| flink.job.checkpoint.count            | cumulative | "{checkpoints}" | checkpoint             | job                     | The number of checkpoints completed or failed.                                                                |
| flink.job.checkpoint.in_progress      | cumulative | "{checkpoints}" |                        | job                     | The number of checkpoints in progress.                                                                        |
| flink.task.record.count               | cumulative | "{records}"     | record                 | task                    | The number of records a task has.                                                                             |
| flink.operator.record.count           | cumulative | "{records}"     | operator_name, record  | operator                | The number of records an operator has.                                                                        |
| flink.operator.watermark.output       | cumulative | ms              | operator_name          | operator                | The last watermark this operator has emitted.                                                                 |

## Logs

Flink logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry) for Client, Jobmanager and Taskmanagers:

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.source` | string | Module and/or thread  where the log originated |
| `jsonPayload.message` | string | Log message, including detailed stacktrace where provided |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Any fields that are blank or missing will not be present in the log entry.
