// Copyright 2021 Google LLC
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

import (
	"fmt"
	"path"
	"strings"
)

// TODO: Move structs out of conf.go

// DBPath returns the database path for the given log tag
func DBPath(tag string) string {
	// TODO: More sanitization?
	dir := strings.ReplaceAll(strings.ReplaceAll(tag, ".", "_"), "/", "_")
	return path.Join("${buffers_dir}", dir)
}

func (i Tail) Component() Component {
	config := map[string]string{
		// https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
		"Name": "tail",
		"Tag":  i.Tag,
		// TODO: Escaping?
		"Path":           strings.Join(i.IncludePaths, ","),
		"DB":             DBPath(i.Tag),
		"Read_from_Head": "True",
		// Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
		"Buffer_Chunk_Size": "512k",
		// Set the max size a bit larger to accommodate for long log lines.
		"Buffer_Max_Size": "5M",
		// When a message is unstructured (no parser applied), append it under a key named "message".
		"Key": "message",
		// Increase this to 30 seconds so log rotations are handled more gracefully.
		"Rotate_Wait": "30",
		// Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
		"Skip_Long_Lines": "On",

		// https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
		// Buffer in disk to improve reliability.
		"storage.type": "filesystem",

		// https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
		// This controls how much data the input plugin can hold in memory once the data is ingested into the core.
		// This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
		// When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
		// as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
		"Mem_Buf_Limit": "10M",
	}
	if len(i.ExcludePaths) > 0 {
		// TODO: Escaping?
		config["Exclude_Path"] = strings.Join(i.ExcludePaths, ",")
	}
	return Component{
		Kind:   "INPUT",
		Config: config,
	}
}

func (i Syslog) Component() Component {
	return Component{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/syslog
			"Name":   "syslog",
			"Tag":    i.Tag,
			"Mode":   i.Mode,
			"Listen": i.Listen,
			"Port":   fmt.Sprintf("%d", i.Port),
			"Parser": "lib:default_message_parser",
			// https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
			// Buffer in disk to improve reliability.
			"storage.type": "filesystem",

			// https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
			// This controls how much data the input plugin can hold in memory once the data is ingested into the core.
			// This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
			// When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
			// as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
			"Mem_Buf_Limit": "10M",
		},
	}
}

func (i WindowsEventlog) Component() Component {
	return Component{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
			"Name":         "winlog",
			"Tag":          i.Tag,
			"Channels":     strings.Join(i.Channels, ","),
			"Interval_Sec": "1",
			"DB":           DBPath(i.Tag),
		},
	}
}
