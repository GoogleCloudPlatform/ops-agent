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
	"strconv"
	"strings"
	"time"

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
	IncludePaths            []string       `yaml:"include_paths,omitempty" validate:"required"`
	ExcludePaths            []string       `yaml:"exclude_paths,omitempty"`
	WildcardRefreshInterval *time.Duration `yaml:"wildcard_refresh_interval,omitempty" validate:"omitempty,min=1s,multipleof_time=1s"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func (r LoggingReceiverFiles) Components(tag string) []fluentbit.Component {
	return LoggingReceiverFilesMixin{
		IncludePaths:            r.IncludePaths,
		ExcludePaths:            r.ExcludePaths,
		WildcardRefreshInterval: r.WildcardRefreshInterval,
	}.Components(tag)
}

type LoggingReceiverFilesMixin struct {
	IncludePaths            []string        `yaml:"include_paths,omitempty"`
	ExcludePaths            []string        `yaml:"exclude_paths,omitempty"`
	WildcardRefreshInterval *time.Duration  `yaml:"wildcard_refresh_interval,omitempty" validate:"omitempty,min=1s,multipleof_time=1s"`
	MultilineRules          []MultilineRule `yaml:"-"`
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
	if r.WildcardRefreshInterval != nil {
		refreshIntervalSeconds := int(r.WildcardRefreshInterval.Seconds())
		config["Refresh_Interval"] = strconv.Itoa(refreshIntervalSeconds)
	}

	c := []fluentbit.Component{}

	if len(r.MultilineRules) > 0 {
		// Configure multiline in the input component;
		// This is necessary, since using the multiline filter will not work
		// if a multiline message spans between two chunks.
		rules := [][2]string{}
		for _, rule := range r.MultilineRules {
			rules = append(rules, [2]string{"rule", rule.AsString()})
		}

		parserName := fmt.Sprintf("multiline.%s", tag)

		c = append(c, fluentbit.Component{
			Kind: "MULTILINE_PARSER",
			Config: map[string]string{
				"name":          parserName,
				"type":          "regex",
				"flush_timeout": "5000",
			},
			OrderedConfig: rules,
		})
		// See https://docs.fluentbit.io/manual/pipeline/inputs/tail#multiline-core-v1.8
		config["multiline.parser"] = parserName

		// multiline parser outputs to a "log" key, but we expect "message" as the output of this pipeline
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Match":  tag,
				"Name":   "modify",
				"Rename": "log message",
			},
		})
	}

	c = append(c, fluentbit.Component{
		Kind:   "INPUT",
		Config: config,
	})

	return c
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

// A LoggingReceiverFluentForward represents the configuration for a Forward Protocol receiver.
type LoggingReceiverFluentForward struct {
	ConfigComponent `yaml:",inline"`

	ListenHost string `yaml:"listen_host,omitempty" validate:"omitempty,ip"`
	ListenPort uint16 `yaml:"listen_port,omitempty"`
}

func (r LoggingReceiverFluentForward) Type() string {
	return "fluent_forward"
}

func (r LoggingReceiverFluentForward) Components(tag string) []fluentbit.Component {
	if r.ListenHost == "" {
		r.ListenHost = "127.0.0.1"
	}
	if r.ListenPort == 0 {
		r.ListenPort = 24224
	}

	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/forward
			"Name":       "forward",
			"Tag_Prefix": tag + ".",
			"Listen":     r.ListenHost,
			"Port":       fmt.Sprintf("%d", r.ListenPort),
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
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverFluentForward{} })
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
	input := []fluentbit.Component{{
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

	// Parser for parsing TimeGenerated field as log record timestamp
	timestampParserName := fmt.Sprintf("%s.timestamp_parser", tag)
	timestampParser := fluentbit.Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Name":        timestampParserName,
			"Format":      "regex",
			"Time_Format": "%Y-%m-%d %H:%M:%S %z",
			"Time_Key":    "timestamp",
			"Regex":       `(?<timestamp>\d+-\d+-\d+ \d+:\d+:\d+ [+-]\d{4})`,
		},
	}

	timestampParserFilter := fluentbit.ParserFilterComponent(tag, "TimeGenerated", []string{timestampParserName})
	timestampParserFilter.Config["Preserve_Key"] = "True"
	timestampParserFilter.Config["Reserve_Data"] = "True"

	input = append(input, timestampParser, timestampParserFilter)

	filters := fluentbit.TranslationComponents(tag, "EventType", "logging.googleapis.com/severity", false,
		[]struct{ SrcVal, DestVal string }{
			{"Error", "ERROR"},
			{"Information", "INFO"},
			{"Warning", "WARNING"},
			{"SuccessAudit", "NOTICE"},
			{"FailureAudit", "NOTICE"},
		})

	return append(input, filters...)
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverWindowsEventLog{} }, "windows")
}

// A LoggingReceiverSystemd represents the user configuration for a Systemd/journald receiver.
type LoggingReceiverSystemd struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverSystemd) Type() string {
	return "systemd_journald"
}

func (r LoggingReceiverSystemd) Components(tag string) []fluentbit.Component {
	input := []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/systemd
			"Name": "systemd",
			"Tag":  tag,
			"DB":   DBPath(tag),
		},
	}}
	filters := fluentbit.TranslationComponents(tag, "PRIORITY", "logging.googleapis.com/severity", false,
		[]struct{ SrcVal, DestVal string }{
			{"7", "DEBUG"},
			{"6", "INFO"},
			{"5", "NOTICE"},
			{"4", "WARNING"},
			{"3", "ERROR"},
			{"2", "CRITICAL"},
			{"1", "ALERT"},
			{"0", "EMERGENCY"},
		})
	input = append(input, filters...)
	input = append(input, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":      "modify",
			"Match":     tag,
			"Condition": fmt.Sprintf("Key_exists %s", "CODE_FILE"),
			"Copy":      fmt.Sprintf("CODE_FILE %s", "logging.googleapis.com/sourceLocation/file"),
		},
	})
	input = append(input, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":      "modify",
			"Match":     tag,
			"Condition": fmt.Sprintf("Key_exists %s", "CODE_FUNC"),
			"Copy":      fmt.Sprintf("CODE_FUNC %s", "logging.googleapis.com/sourceLocation/function"),
		},
	})
	input = append(input, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":      "modify",
			"Match":     tag,
			"Condition": fmt.Sprintf("Key_exists %s", "CODE_LINE"),
			"Copy":      fmt.Sprintf("CODE_LINE %s", "logging.googleapis.com/sourceLocation/line"),
		},
	})
	input = append(input, fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":          "nest",
			"Match":         tag,
			"Operation":     "nest",
			"Wildcard":      "logging.googleapis.com/sourceLocation/*",
			"Nest_under":    "logging.googleapis.com/sourceLocation",
			"Remove_prefix": "logging.googleapis.com/sourceLocation/",
		},
	})
	return input
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverSystemd{} }, "linux")
}

// A LoggingReceiverFluentExec represents the configuration for a Exec Protocol receiver.
type LoggingReceiverFluentExec struct {
	Command []string `yaml:"command,omitempty"`
}

func (r LoggingReceiverFluentExec) Type() string {
	return "fluent_exec"
}

func (r LoggingReceiverFluentExec) Components(tag string) []fluentbit.Component {
	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/exec
			"Name":          "exec",
			"Tag":           tag,
			"Command":       strings.Join(r.Command, " "),
			"Interval_Sec":  "60",
			"Interval_NSec": "0",
			"Buf_Size":      "8mb",
		},
	}}
}

func init() {
	LoggingReceiverTypes.RegisterType(func() Component { return &LoggingReceiverFluentExec{} })
}
