# Vault

Follow [installation guide](https://cloud.google.com/stackdriver/docs/solutions/agents/ops-agent/third-party/vault)
for instructions to collect logs from this application using Ops Agent.

#  `vault_audit` Logging Receiver 

## Prerequisites



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
