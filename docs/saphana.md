# `saphana` Logging Receiver

## Configuration

To configure a receiver for your SAP HANA trace logs, specify the following fields:

| Field                 | Default                       | Description |
| ---                   | ---                           | ---         |
| `type`                | required                      | Must be `saphana`. |
| `include_paths`       | `[/usr/sap/*/HDB*/${HOSTNAME}/trace/*.trc]` | A list of filesystem paths to read by tailing each file. A wild card (`*`) can be used in the paths; for example, `/usr/sap/*/HDB*/${HOSTNAME}/trace/*.trc`.
| `exclude_paths`       | `[/usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver_history*.trc, /usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver*loads*.trc, /usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver*executed_statements*.trc]`                          | A list of filesystem path patterns to exclude from the set matched by `include_paths`.
| `wildcard_refresh_interval` | `60s` | The interval at which wildcard file paths in include_paths are refreshed. Specified as a time interval parsable by [time.ParseDuration](https://pkg.go.dev/time#ParseDuration). Must be a multiple of 1s.|


Example Configuration:

```yaml
logging:
  receivers:
    saphana:
      type: saphana
  service:
    pipelines:
      saphana:
        receivers: [saphana]
```

## Logs

SAP HANA trace logs contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.thread_id` | int | The ID of the thread issuing the log |
| `jsonPayload.connection_id` | int | The ID of the connection associated with the log. If the log's value was "-1", this field will be empty as it is not associated with a connection. |
| `jsonPayload.transaction_id` | int | The ID of the transaction associated with the log. If the log's value was "-1", this field will be empty as it is not associated with a transaction. |
| `jsonPayload.update_transaction_id` | int | The ID of the update transaction associated with the log. If the log's value was "-1", this field will be empty as it is not associated with an update transaction. |
| `jsonPayload.component` | string | The SAP HANA component issuing the log |
| `jsonPayload.source_file` | string | The source location where the log originated |
| `jsonPayload.message` | string | Log message |
| `severity` | string ([`LogSeverity`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity)) | Log entry level (translated) |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the entry was logged |



#[thread_id]{connection_id}[transaction_id/update_transaction_id] timestamp severity_flag component source_file : message