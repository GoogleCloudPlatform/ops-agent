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

package confgenerator

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const addLogNameLuaScriptContents string = `
function split (inputstr, sep)
  if sep == nil then
    sep = "%s"
  end
  local t={}
  for str in string.gmatch(inputstr, "([^"..sep.."]+)") do
    table.insert(t, str)
  end
  return t
end

function add_log_name(tag, timestamp, record)
  -- Split the tag by . to separate the pipeline_id from the rest
  local split_tag = split(tag, '.')
  
  -- The tag is formatted like the following:
  -- <hash_string>.<pipeline_id>.<receiver_id>.<existing_tag>
  --
  -- We can assert that the hash_string, pipeline_id, receiver_id do not
  -- contain the "." delimiter.
  -- We append the existing_tag to the LogName field in the record.
  local receiver_uuid = table.remove(split_tag, 1)
  local pipeline_id = table.remove(split_tag, 1)
  local receiver_id = table.remove(split_tag, 1)
  local existing_tag = table.concat(split_tag, ".")

  -- Replace the record with one with the log name
  record["logging.googleapis.com/logName"] = record["logging.googleapis.com/logName"] .. "." .. existing_tag
  
  -- Use code 2 to replace the original record but keep the original timestamp
  return 2, timestamp, record
end
`

// writeForwardScript writes the above Lua script to the given path so it can be
// used by the logging subagent.
//
// TODO(ridwanmsharif): Replace this with in-config script when
//   fluent/fluent-bit#4634 is supported.
func writeForwardScript(path string) error {
	// Make sure the directory exists before writing the file.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %q: %w", path, err)
	}
	if err := ioutil.WriteFile(path, []byte(addLogNameLuaScriptContents), 0644); err != nil {
		return fmt.Errorf("failed to write file to %q: %w", path, err)
	}
	return nil
}
