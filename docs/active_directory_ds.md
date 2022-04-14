# The `active_directory_ds` Logs Receiver

The `active_directory_ds` logs receiver receives Active Directory logs from the Windows event log.

## Configuration

| Field  | Required | Default | Description                    |
|--------|----------|---------|--------------------------------|
| `type` | required |         | Must be `active_directory_ds`. |

## Example Configuration

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
