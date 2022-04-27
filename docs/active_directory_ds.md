# The `active_directory_ds` Logs Receiver

The `active_directory_ds` logs receiver receives Active Directory logs from the Windows event log.

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

# The `active_directory_ds` Metrics Receiver

The `active_directory_ds` metrics receiver collects performance metrics for an Active Directory Domain Services domain controller.

## Example Configuration

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
