# `modify_fields` Processor

The `modify_fields` processor allows customization of the structure and contents of log entries.

## Configuration structure

```yaml
type: modify_fields:
fields:
  <destination field>:
    # Source
    move_from: <source field>
    copy_from: <source field>
    static_value: <string>
    
    # Mutation
    default_value: <string>
    map_values:
      <old value>: <new value>
    type: {integer|float}
    omit_if: <filter>
```

The top-level configuration for this processor contains a single field,
`fields`, which contains a map of output field names and corresponding
translations.

All transformations are applied in parallel, which means that sources and
filters operate on the original input log entry and may not reference the new
value of any other fields being modified by the same processor.

### Source options

#### No source specified

If no source value is specified, the existing value in the destination field
will be modified.

#### `move_from: <source field>`

The value from `<source field>` will be used as the source for the destination
field. Additionally, `<source field>` will be removed from the log entry. If a
source field is referenced by both `move_from` and `copy_from`, the source field
will still be removed.

#### `copy_from: <source field>`

The value from `<source field>` will be used as the source for the destination
field. `<source field>` will not be removed from the log entry unless it is also
referenced by a `move_from` operation or otherwise modified.

#### `static_value: <string>`

The static string `<string>` will be used as the source for the destination field.

### Mutation options

Zero or more mutation operators may be applied to a single field. If multiple
operators are supplied, they will always be applied in the following order.

#### `default_value: <string>`

If the source field did not exist, the output value will be set to
`<string>`. If the source field already exists (even if it contains an empty
string), the original value is unmodified.

#### `map_values: <map>`

If the input value matches one of the keys in `<map>`, the output value will be
replaced with the corresponding value from the map.

#### `type: {integer|float}`

The input value will be converted to an integer or a float. If the string cannot
be converted to a number, the output value will be unset. If the string contains
a float but the type is specified as `integer`, the number will be truncated to
an integer.

Note that the Google Cloud Logging API uses JSON and therefore it does not
support a full 64-bit integer; if a 64-bit (or larger) integer is needed, it
must be stored as a string in the log entry.

#### `omit_if: <filter>`

If the filter matches the input log record, the output field will be unset. This
can be used to remove placeholder values, such as:

```yaml
httpRequest.referer:
  move_from: jsonPayload.referer
  omit_if: httpRequest.referer = "-"
```

## Sample Configurations

```yaml
receivers:
  in:
    type: files
    include_paths:
    - /var/log/http.json
processors:
  parse_json:
    type: parse_json
  set_http_request:
    type: modify_fields
    fields:
      httpRequest.status:
        move_from: jsonPayload.http_status
        type: integer
      httpRequest.requestUrl:
        move_from: jsonPayload.path
      httpRequest.referer:
        move_from: jsonPayload.referer
        omit_if: jsonPayload.referer = "-"
pipelines:
  pipeline:
    receivers: [in]
    processors: [parse_json, set_http_request]
```

This configuration reads JSON-formatted logs from `/var/log/http.json` and
populates part of the `httpRequest` structure from fields in the logs.
