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
	"strings"
	"time"

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

func (r LoggingReceiverFiles) Expand(_ context.Context) (InternalOTelReceiver, []InternalOTelProcessor) {
	return r.mixin(), nil
}

func (r LoggingReceiverFiles) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	return r.mixin().Pipelines(ctx)
}

type LoggingReceiverFilesMixin struct {
	IncludePaths            []string       `yaml:"include_paths,omitempty"`
	ExcludePaths            []string       `yaml:"exclude_paths,omitempty"`
	WildcardRefreshInterval *time.Duration `yaml:"wildcard_refresh_interval,omitempty" validate:"omitempty,min=1s,multipleof_time=1s"`
	BufferInMemory          bool           `yaml:"-"`
	RecordLogFilePath       *bool          `yaml:"record_log_file_path,omitempty"`
	// In transformation test mode, the file is read exactly once from the beginning, and then the process exits.
	TransformationTest bool `yaml:"-" tracking:"-"`
}

func (r LoggingReceiverFilesMixin) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	operators := []map[string]any{}
	var extensions []string
	receiver_config := map[string]any{
		"include":                       r.IncludePaths,
		"exclude":                       r.ExcludePaths,
		"start_at":                      "beginning",
		"include_file_name":             false,
		"preserve_leading_whitespaces":  true,
		"preserve_trailing_whitespaces": true,
		"fingerprint_size":              "5kb",
	}
	if !r.TransformationTest {
		receiver_config["storage"] = fileStorageExtensionType
		extensions = append(extensions, fileStorageExtensionType)
	}
	if i := r.WildcardRefreshInterval; i != nil {
		receiver_config["poll_interval"] = i.String()
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
	return []otel.ReceiverPipeline{otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "file_log",
			Config: receiver_config,
		},
		Processors: map[string][]otel.Component{
			"logs": nil,
		},
		UsedExtensions: extensions,
	}}, nil
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

func (r LoggingReceiverSyslog) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	body := ottl.LValue{"body"}
	bodyMessage := ottl.LValue{"body", "message"}
	cacheBodyString := ottl.LValue{"cache", "__body_string"}
	cacheBodyMap := ottl.LValue{"cache", "__body_map"}
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
				// Clear cache.
				cacheBodyMap.Delete(),
				cacheBodyString.Delete(),
			),
		),
	}

	config := map[string]any{
		r.TransportProtocol: map[string]any{
			"listen_address": fmt.Sprintf("%s:%d", r.ListenHost, r.ListenPort),
		},
		"protocol": "rfc5424",
	}

	return []otel.ReceiverPipeline{otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "syslog",
			Config: config,
		},
		Processors: map[string][]otel.Component{
			"logs": processors,
		},
	}}, nil
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverSyslog{} })
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

func (r LoggingReceiverFluentForward) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	body := ottl.LValue{"body"}
	bodyMessage := ottl.LValue{"body", "message"}
	attributes := ottl.LValue{"attributes"}
	cacheBodyString := ottl.LValue{"cache", "body_string"}

	processors := []otel.Component{
		otel.Transform(
			"log", "log",
			// Transformations required to convert "fluentforwardreceiver" output to the expected ops agent "fluent_forward" LogEntry format.
			// In summary, this moves all resulting "fluentforwardreceiver" fields into "body" (jsonPayload).
			// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/release/v0.136.x/receiver/fluentforwardreceiver/conversion.go#L171
			ottl.NewStatements(
				// "fluentforwardreceiver" sets "log" and "message" as "body". All other fields are set as "attributes".
				cacheBodyString.SetIf(body, body.IsString()),
				// Merge "cache['body_string']" and "attributes" into "body" (jsonPayload).
				body.Set(ottl.RValue("{}")),
				bodyMessage.SetIf(cacheBodyString, cacheBodyString.IsPresent()),
				body.MergeMaps(attributes, "upsert"),
				attributes.Set(ottl.RValue("{}")),
			),
		),
	}

	return []otel.ReceiverPipeline{otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type: "fluentforward",
			Config: map[string]any{
				"endpoint": fmt.Sprintf("%s:%d", r.ListenHost, r.ListenPort),
			},
		},
		Processors: map[string][]otel.Component{
			"logs": processors,
		},
	}}, nil
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

func (r LoggingReceiverWindowsEventLog) Type() string {
	return "windows_event_log"
}

func (r LoggingReceiverWindowsEventLog) IsDefaultVersion() bool {
	return r.ReceiverVersion == "" || r.ReceiverVersion == "1"
}

func (r LoggingReceiverWindowsEventLog) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	var out []otel.ReceiverPipeline
	for _, c := range r.Channels {
		// https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/release/v0.136.x/receiver/windowseventlogreceiver
		receiver_config := map[string]any{
			"channel":               c,
			"start_at":              "beginning",
			"event_data_format":     "array",
			"poll_interval":         "1s",
			"ignore_channel_errors": true,
			"storage":               fileStorageExtensionType,
			// When "include_log_record_original = true", the event original XML string is set in `attributes."log.record.original"`.
			"include_log_record_original": true,
		}

		var p []otel.Component
		var err error
		if r.IsDefaultVersion() {
			p, err = windowsEventLogV1Processors(ctx)
		} else if r.RenderAsXML {
			p, err = windowsEventLogRawXMLProcessors(ctx)
		} else {
			p, err = windowsEventLogV2Processors(ctx)
		}
		if err != nil {
			return nil, err
		}

		out = append(out, otel.ReceiverPipeline{
			Receiver: otel.Component{
				Type:   "windowseventlog",
				Config: receiver_config,
			},
			Processors: map[string][]otel.Component{
				"logs": p,
			},
			UsedExtensions: []string{fileStorageExtensionType},
		})
	}
	return out, nil
}

// LoggingProcessorWindowsEventLogV1 contains the otel logging processors for ReceiverVersion=1.
type LoggingProcessorWindowsEventLogV1 struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingProcessorWindowsEventLogV1) Type() string {
	return "windows_event_log_v1"
}

func (p LoggingProcessorWindowsEventLogV1) Processors(ctx context.Context) ([]otel.Component, error) {
	return windowsEventLogV1Processors(ctx)
}

func parseLogRecordOriginal(deleteOriginalField bool) otel.Component {
	// Parse original XML (attributes."log.record.original") to preserve non-rendered `Event.System` fields and non-parsed `Event.RenderingInfo.Message`.
	logRecordOriginal := ottl.LValue{"attributes", "log.record.original"}
	bodyParsedXML := ottl.LValue{"body", "parsed_xml"}
	statements := []ottl.Statements{
		bodyParsedXML.SetIf(ottl.ParseSimplifiedXML(logRecordOriginal), logRecordOriginal.IsPresent()),
	}
	if deleteOriginalField {
		statements = append(statements, logRecordOriginal.Delete())
	}
	return otel.Transform(
		"log", "log",
		ottl.NewStatements(statements...),
	)
}

func windowsEventLogV1Processors(ctx context.Context) ([]otel.Component, error) {
	// The winlog input in fluent-bit has a completely different structure.
	// We need to convert the OTel format into the fluent-bit format.
	processors := []otel.Component{parseLogRecordOriginal(true)}

	var empty string
	modifyFields := &LoggingProcessorModifyFields{
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
			"jsonPayload.EventCategory": {CopyFrom: "jsonPayload.parsed_xml.Event.System.Task", Type: "integer"},
			"jsonPayload.EventID":       {CopyFrom: "jsonPayload.event_id.id"},
			"jsonPayload.EventType": {
				CopyFrom: "jsonPayload.level",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					keywords := ottl.LValue{"cache", "body", "keywords"}
					return ottl.NewStatements(
						v.SetIf(ottl.StringLiteral("SuccessAudit"), ottl.ContainsValue(keywords, "Audit Success")),
						v.SetIf(ottl.StringLiteral("FailureAudit"), ottl.ContainsValue(keywords, "Audit Failure")),
					)
				},
			},
			"jsonPayload.Message":      {CopyFrom: "jsonPayload.parsed_xml.Event.RenderingInfo.Message"},
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
					return v.SetIf(ottl.ToValues(v), v.IsPresent())
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

	p, err := modifyFields.Processors(ctx)
	if err != nil {
		return nil, err
	}
	processors = append(processors, p...)
	return processors, nil
}

// LoggingProcessorWindowsEventLogV2 contains the otel logging processors for ReceiverVersion=2.
type LoggingProcessorWindowsEventLogV2 struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingProcessorWindowsEventLogV2) Type() string {
	return "windows_event_log_v2"
}

func (p LoggingProcessorWindowsEventLogV2) Processors(ctx context.Context) ([]otel.Component, error) {
	return windowsEventLogV2Processors(ctx)
}

func windowsEventLogV2Processors(ctx context.Context) ([]otel.Component, error) {
	// The winevtlog input in fluent-bit has a completely different structure.
	// We need to convert the OTel format into the fluent-bit format.
	processors := []otel.Component{parseLogRecordOriginal(true)}

	var empty string
	var zero string = "0"
	modifyFields := &LoggingProcessorModifyFields{
		EmptyBody: true,
		Fields: map[string]*ModifyField{
			"jsonPayload.Channel":       {CopyFrom: "jsonPayload.channel", DefaultValue: &empty},
			"jsonPayload.Computer":      {CopyFrom: "jsonPayload.computer", DefaultValue: &empty},
			"jsonPayload.EventID":       {CopyFrom: "jsonPayload.event_id.id", Type: "integer", DefaultValue: &zero},
			"jsonPayload.EventRecordID": {CopyFrom: "jsonPayload.record_id", Type: "integer", DefaultValue: &zero},
			"jsonPayload.Keywords":      {CopyFrom: "jsonPayload.parsed_xml.Event.System.Keywords"},
			"jsonPayload.Level":         {CopyFrom: "jsonPayload.parsed_xml.Event.System.Level", Type: "integer", DefaultValue: &zero},
			"jsonPayload.Message":       {CopyFrom: "jsonPayload.parsed_xml.Event.RenderingInfo.Message", DefaultValue: &empty},
			"jsonPayload.Opcode":        {CopyFrom: "jsonPayload.parsed_xml.Event.System.Opcode", Type: "integer", DefaultValue: &zero},
			"jsonPayload.ProcessID":     {CopyFrom: "jsonPayload.execution.process_id", Type: "integer", DefaultValue: &zero},
			"jsonPayload.ProviderGuid":  {CopyFrom: "jsonPayload.provider.guid", DefaultValue: &empty},
			"jsonPayload.ProviderName":  {CopyFrom: "jsonPayload.provider.name", DefaultValue: &empty},
			"jsonPayload.Qualifiers":    {CopyFrom: "jsonPayload.event_id.qualifiers", Type: "integer", DefaultValue: &zero},
			"jsonPayload.StringInserts": {
				CopyFrom: "jsonPayload.event_data",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					eventData := ottl.LValue{v.String(), "data"}
					eventBinary := ottl.LValue{v.String(), "binary"}
					cacheEventData := ottl.LValue{"cache", "__event_data"}
					return ottl.NewStatements(
						cacheEventData.SetIf(ottl.ToValues(eventData), eventData.IsPresent()),
						cacheEventData.AppendValuesIf(eventBinary, ottl.And(cacheEventData.IsPresent(), eventBinary.IsPresent())),
						v.SetIf(cacheEventData, cacheEventData.IsPresent()),
					)
				},
			},
			"jsonPayload.Task":     {CopyFrom: "jsonPayload.parsed_xml.Event.System.Task", Type: "integer", DefaultValue: &zero},
			"jsonPayload.ThreadId": {CopyFrom: "jsonPayload.execution.thread_id", Type: "integer", DefaultValue: &zero},
			"jsonPayload.TimeCreated": {
				CopyFrom:          "jsonPayload.system_time",
				CustomConvertFunc: formatSystemTime,
			},
			"jsonPayload.UserId": {
				CopyFrom:     "jsonPayload.security.user_id",
				DefaultValue: &empty,
			},
			"jsonPayload.ActivityID":        {CopyFrom: "jsonPayload.correlation.activity_id", DefaultValue: &empty},
			"jsonPayload.RelatedActivityID": {CopyFrom: "jsonPayload.correlation.related_activity_id", DefaultValue: &empty},
			"jsonPayload.Version":           {CopyFrom: "jsonPayload.version", Type: "integer", DefaultValue: &zero},
		}}
	p, err := modifyFields.Processors(ctx)
	if err != nil {
		return nil, err
	}
	processors = append(processors, p...)
	return processors, nil
}

// LoggingProcessorWindowsEventLogRawXML contains the otel logging processors for RenderAsXML=true.
type LoggingProcessorWindowsEventLogRawXML struct {
	ConfigComponent `yaml:",inline"`
}

func (r LoggingProcessorWindowsEventLogRawXML) Type() string {
	return "windows_event_log_raw_xml"
}

func (p LoggingProcessorWindowsEventLogRawXML) Processors(ctx context.Context) ([]otel.Component, error) {
	return windowsEventLogRawXMLProcessors(ctx)
}

func windowsEventLogRawXMLProcessors(ctx context.Context) ([]otel.Component, error) {
	// When setting "Render_Event_As_XML = True" in fluent-bit winlog receiver, the resulting log contains
	// the fields "Message", "System" (raw_xml) and "StringInserts". We replicate that structure in otel
	// by setting `include_log_record_original: true` which sets `labels."log.record.original"` with the
	// event original XML.
	processors := []otel.Component{parseLogRecordOriginal(false)}

	var empty string
	modifyFields := &LoggingProcessorModifyFields{
		EmptyBody: true,
		Fields: map[string]*ModifyField{
			"jsonPayload.Message": {CopyFrom: "jsonPayload.parsed_xml.Event.RenderingInfo.Message", DefaultValue: &empty},
			`jsonPayload.raw_xml`: {MoveFrom: `labels."log.record.original"`},
			"jsonPayload.StringInserts": {
				CopyFrom: "jsonPayload.event_data",
				CustomConvertFunc: func(v ottl.LValue) ottl.Statements {
					eventData := ottl.LValue{v.String(), "data"}
					eventBinary := ottl.LValue{v.String(), "binary"}
					cacheEventData := ottl.LValue{"cache", "__event_data"}
					return ottl.NewStatements(
						cacheEventData.SetIf(ottl.ToValues(eventData), eventData.IsPresent()),
						cacheEventData.AppendValuesIf(eventBinary, ottl.And(cacheEventData.IsPresent(), eventBinary.IsPresent())),
						v.SetIf(cacheEventData, cacheEventData.IsPresent()),
					)
				},
			},
		},
	}
	p, err := modifyFields.Processors(ctx)
	if err != nil {
		return nil, err
	}
	processors = append(processors, p...)
	return processors, nil
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

func (r LoggingReceiverSystemd) Pipelines(ctx context.Context) ([]otel.ReceiverPipeline, error) {
	receiver_config := map[string]any{
		"start_at": "beginning",
		"priority": "debug",
		"storage":  fileStorageExtensionType,
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

	return []otel.ReceiverPipeline{otel.ReceiverPipeline{
		Receiver: otel.Component{
			Type:   "journald",
			Config: receiver_config,
		},
		Processors: map[string][]otel.Component{
			"logs": modify_fields_processors,
		},
		UsedExtensions: []string{fileStorageExtensionType},
	}}, nil
}

func init() {
	LoggingReceiverTypes.RegisterType(func() LoggingReceiver { return &LoggingReceiverSystemd{} }, platform.Linux)
}
