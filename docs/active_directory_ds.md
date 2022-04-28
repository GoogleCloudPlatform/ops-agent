# The `active_directory_ds` Logs Receiver

The `active_directory_ds` logs receiver receives Active Directory logs from the Windows event log "Directory Service" and "Active Directory Web Services" channels.

## Configuration Options
| Field               | Required | Default | Description                               |
|---------------------|----------|---------|-------------------------------------------|
| type                | required |         | Must be `active_directory_ds`.            |

### Example Configuration

```yaml
logging:
  receivers:
    active_directory_ds:
      type: active_directory_ds
  service:
    pipelines:
      active_directory_ds:
        receivers:
        - active_directory_ds
```

## Log Fields

The following fields are collected from the Windows Event Logs:
| Field                     | Type                                                                                                                            | Description                                                                                                         |
|---------------------------|---------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| jsonPayload.Message       | string                                                                                                                          | The log message.                                                                                                    |
| jsonPayload.RecordNumber  | number                                                                                                                          | The sequence number of the event log.                                                                               |
| jsonPayload.TimeGenerated | string                                                                                                                          | A timestamp representing when the record was generated.                                                             |
| jsonPayload.TimeWritten   | string                                                                                                                          | A timestamp representing when the record was written to the event log.                                              |
| jsonPayload.EventID       | number                                                                                                                          | An ID identifying the type of the event.                                                                            |
| jsonPayload.EventType     | string                                                                                                                          | The type of event.                                                                                                  |
| jsonPayload.Qualifiers    | number                                                                                                                          | A qualifier number that is used for event identification.                                                           |
| jsonPayload.EventCategory | number                                                                                                                          | The category of the event.                                                                                          |
| jsonPayload.Channel       | string                                                                                                                          | The event log channel where the log was logged.                                                                     |
| jsonPayload.Sid           | string                                                                                                                          | The security identifier identifying a security principal or security group of the process that logged this message. |
| jsonPayload.SourceName    | string                                                                                                                          | The source component that logged this message.                                                                      |
| jsonPayload.ComputerName  | string                                                                                                                          | The name of the computer from which this log originates.                                                            |
| jsonPayload.Data          | string                                                                                                                          | Extra event-specific data included with the log.                                                                    |
| jsonPayload.StringInserts | []string                                                                                                                        | Dynamic string data that was used to construct the log message.                                                     |
| severity                  | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity))                       | Log entry level (translated)                                                                                        |
| timestamp                 | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged                                                                                           |

# The `active_directory_ds` Metrics Receiver

The `active_directory_ds` metrics receiver collects performance metrics for an Active Directory Domain Services domain controller.

## Configuration Options
| Field               | Required | Default | Description                               |
|---------------------|----------|---------|-------------------------------------------|
| type                | required |         | Must be `active_directory_ds`.            |
| collection_interval | optional | 60s     | A time.Duration value, such as 30s or 5m. |

### Example Configuration

```yaml
metrics:
  receivers:
    active_directory_ds:
      type: active_directory_ds
  service:
    pipelines:
      active_directory_ds:
        receivers:
        - active_directory_ds
```
## Metrics

The following metrics are collected from the Active Directory domain controller:
| Metric                                                            | Data Type  | Unit              | Labels          | Description                                                                                                              |
|-------------------------------------------------------------------|------------|-------------------|-----------------|--------------------------------------------------------------------------------------------------------------------------|
| active_directory.ds.replication.network.io                        | CUMULATIVE | By                | direction, type | The amount of network data transmitted by the Directory Replication Agent.                                               |
| active_directory.ds.replication.sync.object.pending               | GAUGE      | {objects}         |                 | The number of objects remaining until the full sync completes for the Directory Replication Agent.                       |
| active_directory.ds.replication.sync.request.count                | CUMULATIVE | {requests}        | result          | The number of sync requests made by the Directory Replication Agent.                                                     |
| active_directory.ds.replication.object.rate                       | GAUGE      | {objects}/s       | direction       | The number of objects transmitted by the Directory Replication Agent per second.                                         |
| active_directory.ds.replication.property.rate                     | GAUGE      | {properties}/s    | direction       | The number of properties transmitted by the Directory Replication Agent per second.                                      |
| active_directory.ds.replication.value.rate                        | GAUGE      | {values}/s        | direction       | The number of values transmitted by the Directory Replication Agent per second.                                          |
| active_directory.ds.replication.operation.pending                 | GAUGE      | {operations}      | type            | The number of pending replication operations for the Directory Replication Agent.                                        |
| active_directory.ds.operation.rate                                | GAUGE      | {operations}/s    |                 | The number of operations performed per second.                                                                           |
| active_directory.ds.name_cache.hit_rate                           | GAUGE      | %                 |                 | The percentage of directory object name component lookups that are satisfied by the Directory System Agent's name cache. |
| active_directory.ds.notification.queued                           | GAUGE      | {notifications}   |                 | The number of pending update notifications that have been queued to push to clients.                                     |
| active_directory.ds.security_descriptor_propagations_event.queued | GAUGE      | {events}          |                 | The number of security descriptor propagation events that are queued for processing.                                     |
| active_directory.ds.suboperation.rate                             | GAUGE      | {suboperations}/s | type            | The rate of sub-operations performed.                                                                                    |
| active_directory.ds.bind.rate                                     | GAUGE      | {binds}/s         |                 | The number of binds per second serviced by this domain controller.                                                       |
| active_directory.ds.thread.count                                  | GAUGE      | {threads}         |                 | The number of threads in use by the directory service.                                                                   |
| active_directory.ds.ldap.client.session.count                     | GAUGE      | {sessions}        |                 | The number of connected LDAP client sessions.                                                                            |
| active_directory.ds.ldap.bind.last_successful.time                | GAUGE      | ms                |                 | The amount of time taken for the last successful LDAP bind.                                                              |
| active_directory.ds.ldap.bind.rate                                | GAUGE      | {binds}/s         |                 | The number of successful LDAP binds per second.                                                                          |
| active_directory.ds.ldap.search.rate                              | GAUGE      | {searches}/s      |                 | The number of LDAP searches per second.                                                                                  |
