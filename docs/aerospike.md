# aerospikereceiver

## Metrics

These are the metrics available for this scraper.

| Name | Description | Unit | Type | Attributes |
| ---- | ----------- | ---- | ---- | ---------- |
| **aeropsike.namespace.transaction.count** | Number of transactions performed on the namespace Aggregate of Aerospike Metrics client_delete_error, client_delete_filtered_out, client_delete_not_found, client_delete_success, client_delete_timeout, client_read_error, client_read_filtered_out, client_read_not_found, client_read_success, client_read_timeout, client_udf_error, client_udf_filtered_out, client_udf_not_found, client_udf_success, client_udf_timeout, client_write_error, client_write_filtered_out, client_write_not_found, client_write_success, client_write_timeout | {transactions} | Sum(Int) | <ul> <li>transaction_type</li> <li>transaction_result</li> </ul> |
| **aerospike.namespace.disk.available** | Minimum percentage of contiguous disk space free to the namespace across all devices | % | Gauge(Int) | <ul> </ul> |
| **aerospike.namespace.memory.free** | Percentage of the namespace's memory which is still free Aerospike metric memory_free_pct | % | Gauge(Int) | <ul> </ul> |
| **aerospike.namespace.memory.usage** | Memory currently used by each component of the namespace Aggregate of Aerospike Metrics memory_used_data_bytes, memory_used_index_bytes, memory_used_set_index_bytes, memory_used_sindex_bytes | By | Sum(Int) | <ul> <li>namespace_component</li> </ul> |
| **aerospike.namespace.scan.count** | Number of scan operations performed on the namespace Aggregate of Aerospike Metrics scan_aggr_abort, scan_aggr_complete, scan_aggr_error, scan_basic_abort, scan_basic_complete, scan_basic_error, scan_ops_bg_abort, scan_ops_bg_complete, scan_ops_bg_error, scan_udf_bg_abort, scan_udf_bg_complete, scan_udf_bg_error | {scans} | Sum(Int) | <ul> <li>scan_type</li> <li>scan_result</li> </ul> |
| **aerospike.node.connection.count** | Number of connections opened and closed to the node Aggregate of Aerospike Metrics client_connections_closed, client_connections_opened, fabric_connections_closed, fabric_connections_opened, heartbeat_connections_closed, heartbeat_connections_opened | {connections} | Sum(Int) | <ul> <li>connection_type</li> <li>connection_op</li> </ul> |
| **aerospike.node.connection.open** | Current number of open connections to the node Aggregate of Aerospike Metrics client_connections, fabric_connections, heartbeat_connections | {connections} | Sum(Int) | <ul> <li>connection_type</li> </ul> |
| **aerospike.node.memory.free** | Percentage of the node's memory which is still free Aerospike Metric system_free_mem_pct | % | Gauge(Int) | <ul> </ul> |

**Highlighted metrics** are emitted by default. Other metrics are optional and not emitted by default.
Any metric can be enabled or disabled with the following scraper configuration:

```yaml
metrics:
  <metric_name>:
    enabled: <true|false>
```

## Resource attributes

| Name | Description | Type |
| ---- | ----------- | ---- |
| aerospike.namespace | Name of the Aerospike namespace | String |
| aerospike.node.name | Name of the Aerospike node collected from | String |

## Metric attributes

| Name | Description | Values |
| ---- | ----------- | ------ |
| connection_op (operation) | Operation performed with a connection (open or close) | close, open |
| connection_type (type) | Type of connection to an Aerospike node | client, fabric, heartbeat |
| namespace_component (component) | Individual component of a namespace | data, index, set_index, secondary_index |
| scan_result (result) | Result of a scan operation performed on a namespace | abort, complete, error |
| scan_type (type) | Type of scan operation performed on a namespace | aggregation, basic, ops_background, udf_background |
| transaction_result (result) | Result of a transaction performed on a namespace | error, filtered_out, not_found, success, timeout |
| transaction_type (type) | Type of transaction performed on a namespace | delete, read, udf, write |
