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
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit/modify"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
	"github.com/GoogleCloudPlatform/ops-agent/internal/platform"
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
	IncludePaths            []string       `yaml:"include_paths" validate:"required,min=1"`
	ExcludePaths            []string       `yaml:"exclude_paths,omitempty"`
	WildcardRefreshInterval *time.Duration `yaml:"wildcard_refresh_interval,omitempty" validate:"omitempty,min=1s,multipleof_time=1s"`
	RecordLogFilePath       *bool          `yaml:"record_log_file_path,omitempty"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func (r LoggingReceiverFiles) mixin() LoggingReceiverFilesMixin {
	return LoggingReceiverFilesMixin{
		IncludePaths:            r.IncludePaths,
		ExcludePaths:            r.ExcludePaths,
		WildcardRefreshInterval: r.WildcardRefreshInterval,
		RecordLogFilePath:       r.RecordLogFilePath,
	}
}

func (r LoggingReceiverFiles) Expand(_ context.Context) (InternalLoggingReceiver, []InternalLoggingProcessor) {
	return r.mixin(), nil
}

func (r LoggingReceiverFiles) Components(ctx context.Context, tag string) []fluentbit.Component {
	return r.mixin().Components(ctx, tag)
}

func (r LoggingReceiverFiles) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	return r.mixin().Pipelines(ctx)
}

type LoggingReceiverFilesMixin struct {
	IncludePaths            []string        `yaml:"include_paths,omitempty"`
	ExcludePaths            []string        `yaml:"exclude_paths,omitempty"`
	WildcardRefreshInterval *time.Duration  `yaml:"wildcard_refresh_interval,omitempty" validate:"omitempty,min=1s,multipleof_time=1s"`
	MultilineRules          []MultilineRule `yaml:"-"`
	BufferInMemory          bool            `yaml:"-"`
	RecordLogFilePath       *bool           `yaml:"record_log_file_path,omitempty"`
	// In transformation test mode, the file is read exactly once from the beginning, and then the process exits.
	TransformationTest bool `yaml:"-" tracking:"-"`
}

const stripNewlineCode = `
local function trim_newline(s)
    -- Check for a Windows-style carriage return and newline (\r\n)
    if string.sub(s, -2) == "\r\n" then
        return string.sub(s, 1, -3)
    -- Check for a Unix/Linux-style newline (\n)
    elseif string.sub(s, -1) == "\n" then
        return string.sub(s, 1, -2)
    end
    -- If no trailing newline is found, return the original string
    return s
end
function strip_newline(tag, timestamp, record)
  record["message"] = trim_newline(record["message"])
  return 2, timestamp, record
end
`

func (r LoggingReceiverFilesMixin) Components(ctx context.Context, tag string) []fluentbit.Component {
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
		"Read_from_Head": "True",
		// Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
		"Buffer_Chunk_Size": "512k",
		// Set the max size a bit larger to accommodate for long log lines.
		"Buffer_Max_Size": "2M",
		// When a message is unstructured (no parser applied), append it under a key named "message".
		"Key": "message",
		// Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
		"Skip_Long_Lines": "On",
	}
	if r.TransformationTest {
		// Transformation tests exit as soon as the log is fully processed.
		config["Exit_On_Eof"] = "True"
	}
	if !r.TransformationTest {
		config["DB"] = DBPath(tag)
		// DB.locking specifies that the database will be accessed only by Fluent Bit.
		// Enabling this feature helps to increase performance when accessing the database
		// but it restrict any external tool to query the content.
		config["DB.locking"] = "true"
		// Increase this to 30 seconds so log rotations are handled more gracefully.
		config["Rotate_Wait"] = "30"
		// https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
		// Buffer in disk to improve reliability.
		config["storage.type"] = "filesystem"

		// https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
		// This controls how much data the input plugin can hold in memory once the data is ingested into the core.
		// This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
		// When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
		// as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
		config["Mem_Buf_Limit"] = "10M"
	}
	if len(r.ExcludePaths) > 0 {
		// TODO: Escaping?
		config["Exclude_Path"] = strings.Join(r.ExcludePaths, ",")
	}
	if r.WildcardRefreshInterval != nil {
		refreshIntervalSeconds := int(r.WildcardRefreshInterval.Seconds())
		config["Refresh_Interval"] = strconv.Itoa(refreshIntervalSeconds)
	}

	if r.RecordLogFilePath != nil && *r.RecordLogFilePath == true {
		config["Path_Key"] = "agent.googleapis.com/log_file_path"
	}

	if r.BufferInMemory {
		config["storage.type"] = "memory"
	}

	c := []fluentbit.Component{}

	if len(r.MultilineRules) > 0 {
		// Configure multiline in the input component;
		// This is necessary, since using the multiline filter will not work
		// if a multiline message spans between two chunks.
		parserName := fmt.Sprintf("multiline.%s", tag)

		c = append(c,
			fluentbit.ParseMultilineComponent(parserName, r.MultilineRules),
		)
		// See https://docs.fluentbit.io/manual/pipeline/inputs/tail#multiline-core-v1.8
		config["multiline.parser"] = parserName

		// multiline parser outputs to a "log" key, but we expect "message" as the output of this pipeline
		c = append(c, modify.NewRenameOptions("log", "message").Component(tag))
		// N.B. multiline parsers generate a trailing newline when used with tail that they *don't* generate when used as a filter
		// https://github.com/fluent/fluent-bit/issues/4227
		// https://github.com/fluent/fluent-bit/issues/8914
		// https://github.com/fluent/fluent-bit/issues/9660
		// Using a regex to remove the newline segfaults fluent-bit (sigh)
		c = append(c, fluentbit.LuaFilterComponents(
			tag,
			"strip_newline",
			stripNewlineCode,
		)...)
	}

	c = append(c, fluentbit.Component{
		Kind:   "INPUT",
		Config: config,
	})

	return c
}

func (r LoggingReceiverFilesMixin) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	operators := []map[string]any{}
	receiver_config := map[string]any{
		"include":                       r.IncludePaths,
		"exclude":                       r.ExcludePaths,
		"start_at":                      "beginning",
		"include_file_name":             false,
		"preserve_leading_whitespaces":  true,
		"preserve_trailing_whitespaces": true,
	}
	if i := r.WildcardRefreshInterval; i != nil {
		receiver_config["poll_interval"] = i.String()
	}
	// TODO: Configure `storage` to store file checkpoints
	if len(r.MultilineRules) > 0 {
		return nil, fmt.Errorf("setting multiline rules in otel filelog receiver is not supported")
	}
	// TODO: Support BufferInMemory
	// OTel parses the log to `body` by default; put it in a `message` field to match fluent-bit's behavior.
	operators = append(operators, map[string]any{
		"id":   "body",
		"type": "move",
		"from": "body",
		"to":   "body.message",
	})
	if r.RecordLogFilePath != nil && *r.RecordLogFilePath {
		receiver_config["include_file_path"] = true
		operators = append(operators, map[string]any{
			"id":   "record_log_file_path",
			"type": "move",
			"from": `attributes["log.file.path"]`,
			"to":   `attributes["agent.googleapis.com/log_file_path"]`,
		})
	}
	receiver_config["operators"] = operators
	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "filelog",
			Config: receiver_config,
		},
		Processors: map[string][]otel.Component{
			"logs": nil,
		},
		ExporterTypes: map[string]otel.ExporterType{
			"logs": otel.OTel,
		},
	}}, nil
}

func (r LoggingReceiverFilesMixin) MergeInternalLoggingProcessor(p InternalLoggingProcessor) (InternalLoggingReceiver, InternalLoggingProcessor) {
	if len(r.MultilineRules) > 0 {
		// Only allow merging once.
		return r, p
	}
	if ep, ok := p.(LoggingProcessorParseMultilineRegex); ok {
		r.MultilineRules = ep.Rules
		ep.Rules = nil
		if len(ep.LoggingProcessorParseRegexComplex.Parsers) == 0 {
			return r, nil
		}
		return r, ep.LoggingProcessorParseRegexComplex
	}
	if ep, ok := p.(*ParseMultiline); ok {
		r.MultilineRules = ep.CombinedRules()
		return r, nil
	}
	return r, p
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverFiles{} })
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

func (r LoggingReceiverSyslog) GetListenPort() uint16 {
	return r.ListenPort
}

func (r LoggingReceiverSyslog) Components(ctx context.Context, tag string) []fluentbit.Component {
	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/syslog
			"Name":   "syslog",
			"Tag":    tag,
			"Mode":   r.TransportProtocol,
			"Listen": r.ListenHost,
			"Port":   fmt.Sprintf("%d", r.GetListenPort()),
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

func (r LoggingReceiverSyslog) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	body := ottl.LValue{"body"}
	bodyMessage := ottl.LValue{"body", "message"}
	cacheBodyString := ottl.LValue{"cache", "body_string"}
	cacheBodyMap := ottl.LValue{"cache", "body_map"}
	attributes := ottl.LValue{"attributes"}

	processors := []otel.Component{
		otel.Transform(
			"log", "log",
			// Transformations required to convert "syslogreceiver" output to the expected ops agent "syslog" LogEntry format.
			ottl.NewStatements(
				// "syslogreceiver" sets the incoming log as "body" of type "string".
				cacheBodyString.SetIf(body, body.IsString()),
				// Preserve any existing fields in "body" just in case.
				cacheBodyMap.SetIf(body, body.IsMap()),
				body.Set(ottl.RValue("{}")),
				body.MergeMapsIf(cacheBodyMap, "upsert", cacheBodyMap.IsPresent()),
				// Move "body" to "body.message" (jsonPayload.message)
				bodyMessage.SetIf(cacheBodyString, cacheBodyString.IsPresent()),
				// Clear any possible parsed fields added to "attributes".
				attributes.Set(ottl.RValue("{}")),
			),
		),
	}

	config := map[string]any{
		r.TransportProtocol: map[string]any{
			"listen_address": fmt.Sprintf("%s:%d", r.ListenHost, r.ListenPort),
		},
		"protocol": "rfc5424",
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "syslog",
			Config: config,
		},
		Processors: map[string][]otel.Component{
			"logs": processors,
		},

		ExporterTypes: map[string]otel.ExporterType{
			"logs": otel.OTel,
		},
	}}, nil
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverSyslog{} })
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

func (r LoggingReceiverTCP) GetListenPort() uint16 {
	if r.ListenPort == 0 {
		r.ListenPort = 5170
	}
	return r.ListenPort
}

func (r LoggingReceiverTCP) Components(ctx context.Context, tag string) []fluentbit.Component {
	if r.ListenHost == "" {
		r.ListenHost = "127.0.0.1"
	}

	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/tcp
			"Name":   "tcp",
			"Tag":    tag,
			"Listen": r.ListenHost,
			"Port":   fmt.Sprintf("%d", r.GetListenPort()),
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

			// Allow incoming logs to occupy the maximum possible size per the Logging API (256k).
			// Use a safety factor of 2 to account for things like encoding overhead.
			"Chunk_Size": "512k",
		},
	}}
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverTCP{} })
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

func (r LoggingReceiverFluentForward) GetListenPort() uint16 {
	if r.ListenPort == 0 {
		r.ListenPort = 24224
	}
	return r.ListenPort
}

func (r LoggingReceiverFluentForward) Components(ctx context.Context, tag string) []fluentbit.Component {
	if r.ListenHost == "" {
		r.ListenHost = "127.0.0.1"
	}

	return []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/pipeline/inputs/forward
			"Name":       "forward",
			"Tag_Prefix": tag + ".",
			"Listen":     r.ListenHost,
			"Port":       fmt.Sprintf("%d", r.GetListenPort()),
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
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverFluentForward{} })
}

// A LoggingReceiverWindowsEventLog represents the user configuration for a Windows event log receiver.
type LoggingReceiverWindowsEventLog struct {
	ConfigComponent `yaml:",inline"`

	Channels        []string `yaml:"channels,omitempty,flow" validate:"required,winlogchannels"`
	ReceiverVersion string   `yaml:"receiver_version,omitempty" validate:"omitempty,oneof=1 2" tracking:""`
	RenderAsXML     bool     `yaml:"render_as_xml,omitempty" tracking:""`
}

const eventLogV2SeverityParserLua = `
function process(tag, timestamp, record)
    severityKey = 'logging.googleapis.com/severity'
    if record['Level'] == 1 then
        record[severityKey] = 'CRITICAL'
    elseif record['Level'] == 2 then
        record[severityKey] = 'ERROR'
    elseif record['Level'] == 3 then
        record[severityKey] = 'WARNING'
    elseif record['Level'] == 4 then
        record[severityKey] = 'INFO'
    elseif record['Level'] == 5 then
        record[severityKey] = 'NOTICE'
    end
    return 2, timestamp, record
end
`

func (r LoggingReceiverWindowsEventLog) Type() string {
	return "windows_event_log"
}

func (r LoggingReceiverWindowsEventLog) IsDefaultVersion() bool {
	return r.ReceiverVersion == "" || r.ReceiverVersion == "1"
}

func (r LoggingReceiverWindowsEventLog) Components(ctx context.Context, tag string) []fluentbit.Component {
	if len(r.ReceiverVersion) == 0 {
		r.ReceiverVersion = "1"
	}

	inputName := "winlog"
	timeKey := "TimeGenerated"

	if !r.IsDefaultVersion() {
		inputName = "winevtlog"
		timeKey = "TimeCreated"
	}

	// https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
	input := []fluentbit.Component{{
		Kind: "INPUT",
		Config: map[string]string{
			"Name": inputName,
			"Tag":  tag,
			// TODO(@braydonk): Remove this upon the next Fluent Bit update. See https://github.com/fluent/fluent-bit/issues/8854
			"String_Inserts": "true",
			"Channels":       strings.Join(r.Channels, ","),
			"Interval_Sec":   "1",
			"DB":             DBPath(tag),
		},
	}}

	// On Windows Server 2012/2016, there is a known problem where most log fields end
	// up blank. The Use_ANSI configuration is provided to work around this; however,
	// this also strips Unicode characters away, so we only use it on affected
	// platforms. This only affects the newer API.
	p := platform.FromContext(ctx)
	if !r.IsDefaultVersion() && (p.Is2012() || p.Is2016()) {
		input[0].Config["Use_ANSI"] = "True"
	}

	if r.RenderAsXML {
		input[0].Config["Render_Event_As_XML"] = "True"
		// By default, fluent-bit puts the rendered XML into a field named "System"
		// (this is a constant field name and has no relation to the "System" channel).
		// Rename it to "raw_xml" because it's a more descriptive name than "System".
		input = append(input, modify.NewRenameOptions("System", "raw_xml").Component(tag))
	}

	// Parser for parsing TimeCreated/TimeGenerated field as log record timestamp.
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

	timestampParserFilters := fluentbit.ParserFilterComponents(tag, timeKey, []string{timestampParserName}, true)
	input = append(input, timestampParser)
	input = append(input, timestampParserFilters...)

	var filters []fluentbit.Component
	if r.IsDefaultVersion() {
		filters = fluentbit.TranslationComponents(tag, "EventType", "logging.googleapis.com/severity", false,
			[]struct{ SrcVal, DestVal string }{
				{"Error", "ERROR"},
				{"Information", "INFO"},
				{"Warning", "WARNING"},
				{"SuccessAudit", "NOTICE"},
				{"FailureAudit", "NOTICE"},
			})
	} else {
		// Ordinarily we use fluentbit.TranslationComponents to populate severity,
		// which uses 'modify' filters, except 'modify' filters only work on string
		// values and Level is an int. So we need to use Lua.
		filters = fluentbit.LuaFilterComponents(tag, "process", eventLogV2SeverityParserLua)
	}

	return append(input, filters...)
}

func (r LoggingReceiverWindowsEventLog) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	// TODO: r.IsDefaultVersion() should use the old windows event log API, but the Collector doesn't have a receiver for that.
	var out []otel.ReceiverPipeline
	for _, c := range r.Channels {
		receiver_config := map[string]any{
			"channel":       c,
			"start_at":      "beginning",
			"poll_interval": "1s",
			// TODO: Configure storage
		}
		if r.RenderAsXML {
			receiver_config["raw"] = true
			// TODO: Rename to `jsonPayload.raw_xml`
		}
		var p []otel.Component
		if r.IsDefaultVersion() {
			var err error
			p, err = windowsEventLogV1Processors(ctx)
			if err != nil {
				return nil, err
			}
		}
		// TODO: Add processors for fluent-bit's V2 format.
		out = append(out, otel.ReceiverPipeline{
			Receiver: otel.Component{
				Type:   "windowseventlog",
				Config: receiver_config,
			},
			Processors: map[string][]otel.Component{
				"logs": p,
			},
			ExporterTypes: map[string]otel.ExporterType{
				"logs": otel.OTel,
			},
		})
	}
	return out, nil
}

// LoggingProcessorWindowsEventLogV1 contains the processors for the ReceiverVersion=1.
type LoggingProcessorWindowsEventLogV1 struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingProcessorWindowsEventLogV1) Type() string {
	return "windows_event_log_v1"
}

func (p LoggingProcessorWindowsEventLogV1) Components(ctx context.Context, tag, uid string) []fluentbit.Component {
	// TODO: Refactor LoggingReceiverWindowsEventLog into separate receiver and processor components for transformation tests.
	// Should integrate the configuration for "ReceiverVersion" and "RenderAsXML".
	return []fluentbit.Component{}
}

func (p LoggingProcessorWindowsEventLogV1) Processors(ctx context.Context) ([]otel.Component, error) {
	// TODO: Refactor LoggingReceiverWindowsEventLog into separate receiver and processor components for transformation tests.
	// Should integrate the configuration for "ReceiverVersion" and "RenderAsXML".
	return windowsEventLogV1Processors(ctx)
}

func windowsEventLogV1Processors(ctx context.Context) ([]otel.Component, error) {
	// The winlog input in fluent-bit has a completely different structure, so we need to convert the OTel format into the fluent-bit format.
	var empty string
	p := &LoggingProcessorModifyFields{
		EmptyBody: true,
		Fields: map[string]*ModifyField{
			"jsonPayload.Channel":      {CopyFrom: "jsonPayload.channel"},
			"jsonPayload.ComputerName": {CopyFrom: "jsonPayload.computer"},
			"jsonPayload.Data": {
				CopyFrom:     "jsonPayload.event_data.binary",
				DefaultValue: &empty,
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					return v.Set(ottl.ConvertCase(v, "lower"))
				},
			},
			// TODO: OTel puts the human-readable category at jsonPayload.task, but we need them to add the integer version.
			//"jsonPayload.EventCategory": {StaticValue: "0", Type: "integer"},
			"jsonPayload.EventID": {CopyFrom: "jsonPayload.event_id.id"},
			"jsonPayload.EventType": {
				CopyFrom: "jsonPayload.level",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					// TODO: What if there are multiple keywords?
					keywords := ottl.LValue{"cache", "body", "keywords"}
					keyword0 := ottl.RValue(`cache["body"]["keywords"][0]`)
					return ottl.NewStatements(
						v.SetIf(ottl.StringLiteral("SuccessAudit"), ottl.And(
							keywords.IsPresent(),
							ottl.IsNotNil(keyword0),
							ottl.Equals(keyword0, ottl.StringLiteral("Audit Success")),
						)),
						v.SetIf(ottl.StringLiteral("FailureAudit"), ottl.And(
							keywords.IsPresent(),
							ottl.IsNotNil(keyword0),
							ottl.Equals(keyword0, ottl.StringLiteral("Audit Failure")),
						)),
					)
				},
			},
			// TODO: Fix OTel receiver to provide raw non-parsed messages.
			"jsonPayload.Message":      {CopyFrom: "jsonPayload.message"},
			"jsonPayload.Qualifiers":   {CopyFrom: "jsonPayload.event_id.qualifiers"},
			"jsonPayload.RecordNumber": {CopyFrom: "jsonPayload.record_id"},
			"jsonPayload.Sid": {
				CopyFrom:     "jsonPayload.security.user_id",
				DefaultValue: &empty,
			},
			"jsonPayload.SourceName": {
				CopyFrom: "jsonPayload.provider.name",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					// Prefer jsonPayload.provider.event_source if present and non-empty
					eventSource := ottl.LValue{"cache", "body", "provider", "event_source"}
					return v.SetIf(
						eventSource,
						ottl.And(
							eventSource.IsPresent(),
							ottl.Not(ottl.Equals(
								eventSource,
								ottl.StringLiteral(""),
							)),
						),
					)
				},
			},
			"jsonPayload.StringInserts": {
				CopyFrom: "jsonPayload.event_data.data",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					return v.Set(ottl.ToValues(v))
				},
			},
			"jsonPayload.TimeGenerated": {
				CopyFrom:          "jsonPayload.system_time",
				CustomConvertFunc: formatSystemTime,
			},
			"jsonPayload.TimeWritten": {
				CopyFrom:          "jsonPayload.system_time",
				CustomConvertFunc: formatSystemTime,
			},
		}}
	return p.Processors(ctx)
}

func formatSystemTime(v ottl.LValue) ottl.Statements {
	return v.Set(ottl.FormatTime(ottl.ToTime(v, "%Y-%m-%dT%T.%sZ"), "%Y-%m-%d %T.%s +0000"))
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverWindowsEventLog{} }, platform.Windows)
}

// A LoggingReceiverSystemd represents the user configuration for a Systemd/journald receiver.
type LoggingReceiverSystemd struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverSystemd) Type() string {
	return "systemd_journald"
}

func (r LoggingReceiverSystemd) Components(ctx context.Context, tag string) []fluentbit.Component {
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

func (r LoggingReceiverSystemd) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	receiver_config := map[string]any{
		"start_at": "beginning",
		"priority": "debug",
	}

	modify_fields_processors, err := LoggingProcessorModifyFields{
		Fields: map[string]*ModifyField{
			`severity`: {
				CopyFrom: "jsonPayload.PRIORITY",
				MapValues: map[string]string{
					"7": "DEBUG",
					"6": "INFO",
					"5": "NOTICE",
					"4": "WARNING",
					"3": "ERROR",
					"2": "CRITICAL",
					"1": "ALERT",
					"0": "EMERGENCY",
				},
				MapValuesExclusive: true,
			},
			`sourceLocation.file`: {
				CopyFrom: "jsonPayload.CODE_FILE",
			},
			`sourceLocation.func`: {
				CopyFrom: "jsonPayload.CODE_FUNC",
			},
			`sourceLocation.line`: {
				CopyFrom: "jsonPayload.CODE_LINE",
				Type:     "integer",
			},
		},
	}.Processors(ctx)

	if err != nil {
		return nil, err
	}

	return []otel.ReceiverPipeline{{
		Receiver: otel.Component{
			Type:   "journald",
			Config: receiver_config,
		},
		Processors: map[string][]otel.Component{
			"logs": modify_fields_processors,
		},

		ExporterTypes: map[string]otel.ExporterType{
			"logs": otel.OTel,
		},
	}}, nil
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverSystemd{} }, platform.Linux)
}
