# Vault

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/vault)
for instructions to collect logs from this application using Ops Agent.
# `vault` Metrics Receiver

The `vault` metrics receiver fetches node and cluster level stats from Elasticsearch nodes. The receiver is meant to be run 

## Configurations
To configure a receiver for your Vault metrics, specify the following fields:

| Field                   | Required | Default           | Description |
| ---                     | ---      | ---               | ---         |
| `type`                  | required |                   | Must be `vault`. |
| `endpoint`              | optional | `localhost:8200`  | hostname:port of vault instance to be monitored. |
| `metrics_path`          | optional | `/v1/sys/metrics` | the path for metrics collection. |
| `token`                 | optional |                   | Token used for authentication. |
| `collection_interval`   | optional |                   | A [time.Duration](https://pkg.go.dev/time#ParseDuration) value, such as `30s` or `5m`. |
| `insecure`              | optional | true              | Signals whether to use a secure TLS connection or not. If insecure is true TLS will not be enabled. |
| `insecure_skip_verify`  | optional | false             | Whether to skip verifying the certificate or not. A false value of insecure_skip_verify will not be used if insecure is true as the connection will not use TLS at all. |
| `cert_file`             | optional |                   | Path to the TLS cert to use for mTLS required connections. |
| `key_file`              | optional |                   | Path to the TLS key to use for mTLS required connections. |
| `ca_file`               | optional |                   | Path to the CA cert. As a client this verifies the server certificate. If empty, uses system root CA. |


Example Configuration:

```yaml
metrics:
  receivers:
    vault:
      type: vault
  service:
    pipelines:
      vault:
        receivers:
          - vault
```

## Metrics

The Ops Agent collects the following metrics from your Elasticsearch nodes:

| Metric                                        | Data Type          | Unit          | Labels                  | Description                                                                              |
|-----------------------------------------------|--------------------|---------------|-------------------------|------------------------------------------------------------------------------------------|
| workload.googleapis.com/vault.node.cache.memory.usage              | Gauge (INT64)      | By            | cache_name              | The size in bytes of the cache.                                     |
| workload.googleapis.com/vault.node.cache.evictions                 | Cumulative (INT64) | {evictions}   | cache_name              | The number of evictions from the cache.                             |


Labels:

| Metric Name                                        | Label Name | Description                        | Values |
|----------------------------------------------------|------------|------------------------------------|--------|
| workload.googleapis.com/jvm.gc.collections.count   | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.gc.collections.elapsed | name       | The name of the garbage collector. |        |
| workload.googleapis.com/jvm.memory.pool.max        | name       | The name of the JVM memory pool.   |        |
| workload.googleapis.com/jvm.memory.pool.used       | name       | The name of the JVM memory pool.   |        |


#  `vault_audit` Logging Receiver 

## Logs

Audit logs have variable fields and can contain any subset of these fields. 

| Field | Type | Description |
| ---   | ---- | ----------- |
| `jsonPayload.type` | string | the type of audit log. |
| `jsonPayload.error` | string | If an error occurred with the request, the error message is included in this field’s value. |
| `jsonPayload.auth.client_token` | string | This is an HMAC of the client’s token ID. |
| `jsonPayload.auth.accessor` | string | This is an HMAC of the client token accessor. |
| `jsonPayload.auth.display_name` | string | This is the display name set by the auth method role or explicitly at secret creation time. |
| `jsonPayload.auth.policies` | object | This will contain a list of policies associated with the client_token. |
| `jsonPayload.auth.metadata` | object | This will contain a list of metadata key/value pairs associated with the client_token. |
| `jsonPayload.auth.entity_id` | string | This is a token entity identifier. |
| `jsonPayload.request.id` | string | This is the unique request identifier. |
| `jsonPayload.request.operation` | string | This is the type of operation which corresponds to path capabilities and is expected to be one of: `create`, `read`, `update`, `delete`, or `list`. |
| `jsonPayload.request.client_token` | string | This is an HMAC of the client’s token ID. |
| `jsonPayload.request.client_token_accessor` | string | This is an HMAC of the client token accessor. |
| `jsonPayload.request.path` | string | The requested Vault path for operation. |
| `jsonPayload.request.data` | object | The data object will contain secret data in key/value pairs. |
| `jsonPayload.request.policy_override` | boolean | this is true when a soft-mandatory policy override was requested. |
| `jsonPayload.request.remote_address` | string | The IP address of the client making the request. |
| `jsonPayload.request.wrap_ttl` | string | If the token is wrapped, this displays configured wrapped TTL value as numeric string. |
| `jsonPayload.request.headers` | object | Additional HTTP headers specified by the client as part of the request. |
| `jsonPayload.response.data.creation_time` | string | RFC3339 format timestamp of the token’s creation. |
| `jsonPayload.response.data.creation_ttl` | string | Token creation TTL in seconds. |
| `jsonPayload.response.data.expire_time` | string | RFC3339 format timestamp representing the moment this token will expire. |
| `jsonPayload.response.data.explicit_max_ttl` | string | Explicit token maximum TTL value as seconds (‘0’ when not set). |
| `jsonPayload.response.data.issue_time` | string |  RFC3339 format timestamp. |
| `jsonPayload.response.data.num_uses` | number | If the token is limited to a number of uses, that value will be represented here. |
| `jsonPayload.response.data.orphan` | boolean | Boolean value representing whether the token is an orphan. |
| `jsonPayload.response.data.renewable` | boolean | Boolean value representing whether the token is an orphan. |
| `jsonPayload.response.data.id` | string | This is the unique response identifier. |
| `jsonPayload.response.data.path` | string | The requested Vault path for operation. |
| `jsonPayload.response.data.policies` | object | This will contain a list of policies associated with the client_token. |
| `jsonPayload.response.data.accessor` | string | This is an HMAC of the client token accessor. |
| `jsonPayload.response.data.display_name` | string | This is the display name set by the auth method role or explicitly at secret creation time. |
| `jsonPayload.response.data.display_name` | string | This is the display name set by the auth method role or explicitly at secret creation time. |
| `jsonPayload.response.data.entity_id` | string | This is a token entity identifier. |
| `timestamp` | string ([`Timestamp`](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf#google.protobuf.Timestamp)) | Time the request was received |

Field descriptions taken from https://support.hashicorp.com/hc/en-us/articles/360000995548-Audit-and-Operational-Log-Details.

Any fields that are blank or missing will not be present in the log entry.

Audit logs commonly contain the following fields in the [`LogEntry`](https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry):
