// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fluentbit

const (
	ParserNestLuaFunction = `parser_nest`

	// ParserNestLuaScriptContents is an incomplete Lua funtion that is completed when populated
	// with the parse_key (i.e. the key being parsed).
	ParserNestLuaScriptContents = `
function parser_nest(tag, timestamp, record)
  local nestedRecord = {}
  local parseKey = "%s"
  for k, v in pairs(record) do
      if k ~= parseKey then
          nestedRecord[k] = v
      end
  end

  local result = {}
  result[parseKey] = record[parseKey]
  result["logging.googleapis.com/__tmp"] = nestedRecord

  return 2, timestamp, result
end

`
	ParserMergeLuaFunction       = `parser_merge_record`
	ParserMergeLuaScriptContents = `
  function shallow_merge(record, parsedRecord)
    -- If no exiting record exists
    if (record == nil) then 
        return parsedRecord
    end
    
    for k, v in pairs(parsedRecord) do
        record[k] = v
    end

    return record
end

function merge(record, parsedRecord)
    -- If no exiting record exists
    if record == nil then 
        return parsedRecord
    end
    
    -- Potentially overwrite or merge the original records.
    for k, v in pairs(parsedRecord) do
        -- If there is no conflict
        if k == "logging.googleapis.com/logName" then 
            -- Ignore the parsed payload since the logName is controlled
            -- by the OpsAgent.
        elseif k == "logging.googleapis.com/labels" then 
            -- LogEntry.labels are basically a map[string]string and so only require a
            -- shallow merge (one level deep merge).
            record[k] = shallow_merge(record[k], v)
        else
            record[k] = v
        end
    end

    return record
end

function parser_merge_record(tag, timestamp, record)
    originalPayload = record["logging.googleapis.com/__tmp"]
    if originalPayload == nil then
        return 0, timestamp, record
    end
    
    -- Remove original payload
    record["logging.googleapis.com/__tmp"] = nil
    record = merge(originalPayload, record)
    return 2, timestamp, record
end
`
)
