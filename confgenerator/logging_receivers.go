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

package confgenerator

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingReceiverFiles struct {
	ConfigComponent `yaml:",inline"`

	IncludePaths []string `yaml:"include_paths,omitempty" validate:"required"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func (r LoggingReceiverFiles) Components(tag string) []fluentbit.Component {
	config := map[string]string{
		// https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
		"Name": "tail",
		"Tag":  tag,
		// TODO: Escaping?
		"Path":           strings.Join(r.IncludePaths, ","),
		"DB":             fluentbit.DBPath(tag),
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
	if len(r.ExcludePaths) > 0 {
		// TODO: Escaping?
		config["Exclude_Path"] = strings.Join(r.ExcludePaths, ",")
	}
	return []fluentbit.Component{{
		Kind:   "INPUT",
		Config: config,
	}}
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverFiles{} })
}

type LoggingReceiverSyslog struct {
	ConfigComponent `yaml:",inline"`

	TransportProtocol string `yaml:"transport_protocol,omitempty" validate:"oneof=tcp udp"`
	ListenHost        string `yaml:"listen_host,omitempty" validate:"required,ip"`
	ListenPort        uint16 `yaml:"listen_port,omitempty" validate:"required"`
}

func (r LoggingReceiverSyslog) Type() string {
	return "syslog"
}

func (r LoggingReceiverSyslog) Components(tag string) []fluentbit.Component {
	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/syslog
			"Name":   "syslog",
			"Tag":    tag,
			"Mode":   r.TransportProtocol,
			"Listen": r.ListenHost,
			"Port":   fmt.Sprintf("%d", r.ListenPort),
			"Parser": tag,
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
	}, {
		// FIXME: This is not new, but we shouldn't be disabling syslog protocol parsing by passing a custom Parser - Fluentbit includes builtin syslog protocol support, and we should enable/expose that.
		Kind: "PARSER",
		Config: map[string]string{
			"Name":   tag,
			"Format": "regex",
			"Regex":  `^(?<message>.*)$`,
		},
	}}
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverSyslog{} })
}

type LoggingReceiverWindowsEventLog struct {
	ConfigComponent `yaml:",inline"`

	Channels []string `yaml:"channels,omitempty,flow" validate:"required"`
}

func (r LoggingReceiverWindowsEventLog) Type() string {
	return "windows_event_log"
}

func (r LoggingReceiverWindowsEventLog) Components(tag string) []fluentbit.Component {
	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
			"Name":         "winlog",
			"Tag":          tag,
			"Channels":     strings.Join(r.Channels, ","),
			"Interval_Sec": "1",
			"DB":           fluentbit.DBPath(tag),
		},
	}}
}
func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverWindowsEventLog{} }, "windows")
}
