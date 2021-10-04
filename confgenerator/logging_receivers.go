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
	"path"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// DBPath returns the database path for the given log tag
func DBPath(tag string) string {
	// TODO: More sanitization?
	dir := strings.ReplaceAll(strings.ReplaceAll(tag, ".", "_"), "/", "_")
	return path.Join("${buffers_dir}", dir)
}

// A LoggingReceiverFiles represents the user configuration for a file receiver (fluentbit's tail plugin).
type LoggingReceiverFiles struct {
	ConfigComponent `yaml:",inline"`
	// TODO: Use LoggingReceiverFilesMixin after figuring out the validation story.
	IncludePaths []string `yaml:"include_paths,omitempty" validate:"required"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func (r LoggingReceiverFiles) Components(tag string) []fluentbit.Component {
	return LoggingReceiverFilesMixin{
		IncludePaths: r.IncludePaths,
		ExcludePaths: r.ExcludePaths,
	}.Components(tag)
}

type LoggingReceiverFilesMixin struct {
	IncludePaths []string `yaml:"include_paths,omitempty"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

func (r LoggingReceiverFilesMixin) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		// No files -> no input.
		return nil
	}
	config := map[string]string{
		// https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
		"Name": "tail",
		"Tag":  tag,
		// TODO: Escaping?
		"Path":           strings.Join(r.IncludePaths, ","),
		"DB":             DBPath(tag),
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
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverFiles{} })
}

// A LoggingReceiverSyslog represents the configuration for a syslog protocol receiver.
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
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverSyslog{} })
}

// A LoggingReceiverTCP represents the configuration for a TCP receiver.
type LoggingReceiverTCP struct {
	ConfigComponent `yaml:",inline"`

	Format     string `yaml:"format,omitempty" validate:"required,oneof=json"`
	ListenHost string `yaml:"listen_host,omitempty" validate:"omitempty,ip"`
	ListenPort uint16 `yaml:"listen_port,omitempty"`
}

func (r LoggingReceiverTCP) Type() string {
	return "tcp"
}

func (r LoggingReceiverTCP) Components(tag string) []fluentbit.Component {
	if r.ListenHost == "" {
		r.ListenHost = "127.0.0.1"
	}
	if r.ListenPort == 0 {
		r.ListenPort = 5170
	}

	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/tcp
			"Name":   "tcp",
			"Tag":    tag,
			"Listen": r.ListenHost,
			"Port":   fmt.Sprintf("%d", r.ListenPort),
			"Format": r.Format,
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
	}}
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverTCP{} })
}

// A LoggingReceiverWindowsEventLog represents the user configuration for a Windows event log receiver.
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
			"DB":           DBPath(tag),
		},
	}}
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverWindowsEventLog{} }, "windows")
}

// A LoggingReceiverSystemd represents the user configuration for a Systemd/journald receiver.
type LoggingReceiverSystemd struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverSystemd) Type() string {
	return "systemd"
}

func (r LoggingReceiverSystemd) Components(tag string) []fluentbit.Component {
	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/systemd
			"Name": "systemd",
			"Tag":  tag,
			"DB":   DBPath(tag),
		},
	}}
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverSystemd{} }, "linux")
}
