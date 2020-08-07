package conf

import (
	"testing"

	"github.com/kylelemons/godebug/diff"
)

func TestParserJSON(t *testing.T) {
	tests := []struct {
		parserJSON         ParserJSON
		expectedTailConfig string
	}{
		{
			parserJSON: ParserJSON{
				Name:       "test_name",
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format json
    Time_Key test_time_key
    Time_Format test_time_format`,
		},
		{
			parserJSON: ParserJSON{
				Name:       "test_name",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format json
    Time_Format test_time_format`,
		},
		{
			parserJSON: ParserJSON{
				Name:    "test_name",
				TimeKey: "test_time_key",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format json
    Time_Key test_time_key`,
		},
	}
	for _, tc := range tests {
		got, err := tc.parserJSON.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("ParserJSON %v: ParserJSON.renderConfig() returned unexpected diff (-want +got):\n%s", tc.parserJSON, diff)
		}
	}
}

func TestParserJSONErrors(t *testing.T) {
	tests := []struct {
		parserJSON ParserJSON
	}{
		{
			parserJSON: ParserJSON{},
		},
		{
			parserJSON: ParserJSON{
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
		},
		{
			parserJSON: ParserJSON{
				TimeKey: "test_time_key",
			},
		},
		{
			parserJSON: ParserJSON{
				TimeFormat: "test_time_format",
			},
		},
	}
	for _, tc := range tests {
		if _, err := tc.parserJSON.renderConfig(); err == nil {
			t.Errorf("ParserJSON %v: ParserJSON.renderConfig() succeeded, want error", tc.parserJSON)
		}
	}
}

func TestParserRegex(t *testing.T) {
	tests := []struct {
		parserRegex        ParserRegex
		expectedTailConfig string
	}{
		{
			parserRegex: ParserRegex{
				Name:  "test_name",
				Regex: "test_regex",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format regex
    Regex test_regex`,
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				Regex:      "test_regex",
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format regex
    Regex test_regex
    Time_Key test_time_key
    Time_Format test_time_format`,
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				Regex:      "test_regex",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format regex
    Regex test_regex
    Time_Format test_time_format`,
		},
		{
			parserRegex: ParserRegex{
				Name:    "test_name",
				Regex:   "test_regex",
				TimeKey: "test_time_key",
			},
			expectedTailConfig: `[PARSER]
    Name test_name
    Format regex
    Regex test_regex
    Time_Key test_time_key`,
		},
	}
	for _, tc := range tests {
		got, err := tc.parserRegex.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return

		}
		if diff := diff.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("ParserRegex %v: ParserRegex.renderConfig() returned unexpected diff (-want +got):\n%s", tc.parserRegex, diff)
		}
	}
}

func TestParserRegexErrors(t *testing.T) {
	tests := []struct {
		parserRegex ParserRegex
	}{
		{},
		{
			parserRegex: ParserRegex{
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
		},
		{
			parserRegex: ParserRegex{
				Regex:      "test_regex",
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
		},
	}
	for _, tc := range tests {
		if _, err := tc.parserRegex.renderConfig(); err == nil {
			t.Errorf("ParserRegex %v: ParserRegex.renderConfig() succeeded, want error", tc.parserRegex)
		}
	}
}

func TestTail(t *testing.T) {
	tests := []struct {
		tail               Tail
		expectedTailConfig string
	}{
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5`,
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5`,
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				Parser:          "test_parser",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5
    Parser test_parser`,
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				ExcludePath:     "test_exclude_path",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5
    Exclude_Path test_exclude_path`,
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				ExcludePath:     "test_exclude_path/file1,test_excloud_path/file2",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5
    Exclude_Path test_exclude_path/file1,test_excloud_path/file2`,
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				ExcludePath:     "test_exclude_path",
				Parser:          "test_parser",
				RefreshInterval: 60,
				RotateWait:      5,
			},
			expectedTailConfig: `[INPUT]
    Name tail
    Path test_path
    DB test_db
    Tag test_tag
    Refresh_Interval 60
    Rotate_Wait 5
    Exclude_Path test_exclude_path
    Parser test_parser`,
		},
	}
	for _, tc := range tests {
		got, err := tc.tail.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("Tail %v: ran Tail.renderConfig() returned unexpected diff (-want +got):\n%s", tc.tail, diff)
		}
	}
}

func TestTailErrors(t *testing.T) {
	tests := []struct {
		tail Tail
	}{
		{
			tail: Tail{
				DB:   "test_db",
				Path: "test_path",
			},
		},
		{
			tail: Tail{
				Tag:  "test_tag",
				Path: "test_path",
			},
		},
		{
			tail: Tail{
				Tag: "test_tag",
				DB:  "test_db",
			},
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				RefreshInterval: 0,
				RotateWait:      5,
			},
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				RefreshInterval: 0,
				RotateWait:      5,
			},
		},
		{
			tail: Tail{
				Tag:             "test_tag",
				DB:              "test_db",
				Path:            "test_path",
				RefreshInterval: 60,
				RotateWait:      0,
			},
		},
	}
	for _, tc := range tests {
		if _, err := tc.tail.renderConfig(); err == nil {
			t.Errorf("Tail %v: Tail.renderConfig() succeeded, want error", tc.tail)
		}
	}
}

func TestSyslog(t *testing.T) {
	tests := []struct {
		syslog               Syslog
		expectedSyslogConfig string
	}{
		{
			syslog: Syslog{
				Mode:   "tcp",
				Listen: "0.0.0.0",
				Port:   1234,
				Parser: "syslog-rfc5424",
				Tag:    "test_tag",
			},
			expectedSyslogConfig: `[INPUT]
    Name syslog
    Mode tcp
    Listen 0.0.0.0
    Tag test_tag
    Port 1234
    Parser syslog-rfc5424`,
		},
	}
	for _, tc := range tests {
		got, err := tc.syslog.renderConfig()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.expectedSyslogConfig, got); diff != "" {
			t.Errorf("Tail %v: ran syslog.renderConfig() returned unexpected diff (-want +got):\n%s", tc.syslog, diff)
		}
	}
}

func TestSyslogErrors(t *testing.T) {
	tests := []struct {
		name   string
		syslog Syslog
	}{
		{
			name: "invalide mode",
			syslog: Syslog{
				Mode:   "invalid_mode",
				Listen: "0.0.0.0",
				Port:   1234,
				Parser: "syslog-rfc5424",
				Tag:    "test_tag",
			},
		},
		{
			name: "invalid listen",
			syslog: Syslog{
				Mode:   "tcp",
				Listen: "non-IP",
				Port:   1234,
				Parser: "syslog-rfc5424",
				Tag:    "test_tag",
			},
		},
		{
			name: "invalid port",
			syslog: Syslog{
				Mode:   "tcp",
				Listen: "0.0.0.0",
				Parser: "syslog-rfc5424",
				Tag:    "test_tag",
			},
		},
		{
			name: "invalid parser",
			syslog: Syslog{
				Mode:   "tcp",
				Listen: "0.0.0.0",
				Port:   1234,
				Parser: "invalid_parser",
				Tag:    "test_tag",
			},
		},
		{
			name: "tag not provided",
			syslog: Syslog{
				Mode:   "tcp",
				Listen: "0.0.0.0",
				Port:   1234,
				Parser: "syslog-rfc5424",
			},
		},
	}
	for _, tc := range tests {
		if _, err := tc.syslog.renderConfig(); err == nil {
			t.Errorf("test %q: syslog.renderConfig() succeeded, want error.", tc.name)
		}
	}
}

func TestGenerateFluentBitMainConfig(t *testing.T) {
	tests := []struct {
		name    string
		tails   []*Tail
		syslogs []*Syslog
		want    string
	}{
		{
			name: "zero plugins",
			want: `[SERVICE]
    Flush         5
    Grace         120
    Log_Level     debug
    Log_File      /var/log/ops_agents/logging_agent.log
    Daemon        off
    HTTP_Server   On
    HTTP_Listen   0.0.0.0

[OUTPUT]
    Name  stackdriver
    resource gce_instance
    Match *

`,
		},
		{
			name: "multiple tail and syslog plugins",
			tails: []*Tail{{
				Tag:             "test_tag1",
				DB:              "test_db1",
				Path:            "test_path1",
				RefreshInterval: 60,
				RotateWait:      5,
			}, {
				Tag:             "test_tag2",
				DB:              "test_db2",
				Path:            "test_path2",
				RefreshInterval: 120,
				RotateWait:      10,
			}},
			syslogs: []*Syslog{{
				Mode:   "tcp",
				Listen: "0.0.0.0",
				Port:   1234,
				Parser: "syslog-rfc5424",
				Tag:    "test_tag1",
			}, {
				Mode:   "udp",
				Listen: "0.0.0.0",
				Port:   5678,
				Parser: "syslog-rfc3164",
				Tag:    "test_tag2",
			}},
			want: `[SERVICE]
    Flush         5
    Grace         120
    Log_Level     debug
    Log_File      /var/log/ops_agents/logging_agent.log
    Daemon        off
    HTTP_Server   On
    HTTP_Listen   0.0.0.0

[OUTPUT]
    Name  stackdriver
    resource gce_instance
    Match *

[INPUT]
    Name tail
    Path test_path1
    DB test_db1
    Tag test_tag1
    Refresh_Interval 60
    Rotate_Wait 5

[INPUT]
    Name tail
    Path test_path2
    DB test_db2
    Tag test_tag2
    Refresh_Interval 120
    Rotate_Wait 10

[INPUT]
    Name syslog
    Mode tcp
    Listen 0.0.0.0
    Tag test_tag1
    Port 1234
    Parser syslog-rfc5424

[INPUT]
    Name syslog
    Mode udp
    Listen 0.0.0.0
    Tag test_tag2
    Port 5678
    Parser syslog-rfc3164

`,
		},
	}
	for _, tc := range tests {
		got, err := GenerateFluentBitMainConfig(tc.tails, tc.syslogs)
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := diff.Diff(tc.want, got); diff != "" {
			t.Errorf("test %q: ran GenerateFluentBitMainConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
		}
	}
}

func TestGenerateFluentBitMainConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		tails   []*Tail
		syslogs []*Syslog
	}{
		{
			name: "an invalid Tail exists",
			tails: []*Tail{{
				DB:   "test_db",
				Path: "test_path",
			},
			},
		},
		{
			name: "an invalid Syslog exists",
			syslogs: []*Syslog{{
				Mode:   "not_syslog",
				Listen: "",
				Port:   0,
				Parser: "not-syslog-parser",
				Tag:    "",
			},
			},
		},
	}
	for _, tc := range tests {
		if _, err := GenerateFluentBitMainConfig(tc.tails, tc.syslogs); err == nil {
			t.Errorf("test %q: GenerateFluentBitMainConfig succeeded, want error", tc.name)
		}
	}
}

func TestGenerateFluentBitParserConfig(t *testing.T) {
	tests := []struct {
		name         string
		jsonParsers  []*ParserJSON
		regexParsers []*ParserRegex
		want         string
	}{
		{
			name: "empty JSON Parsers and Regex Parsers",
			want: `[PARSER]
    Name        syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%z

[PARSER]
    Name        syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

`,
		},
		{
			name: "multiple JSON Parsers and Regex Parsers",
			jsonParsers: []*ParserJSON{
				{
					Name:       "test_name1",
					TimeKey:    "test_time_key1",
					TimeFormat: "test_time_format1",
				}, {
					Name:       "test_name2",
					TimeKey:    "test_time_key2",
					TimeFormat: "test_time_format2",
				},
			},
			regexParsers: []*ParserRegex{{
				Name:  "test_name1",
				Regex: "test_regex1",
			}, {
				Name:  "test_name2",
				Regex: "test_regex2",
			}},
			want: `[PARSER]
    Name        syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%z

[PARSER]
    Name        syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

[PARSER]
    Name test_name1
    Format json
    Time_Key test_time_key1
    Time_Format test_time_format1

[PARSER]
    Name test_name2
    Format json
    Time_Key test_time_key2
    Time_Format test_time_format2

[PARSER]
    Name test_name1
    Format regex
    Regex test_regex1

[PARSER]
    Name test_name2
    Format regex
    Regex test_regex2

`,
		},
	}
	for _, tc := range tests {
		got, err := GenerateFluentBitParserConfig(tc.jsonParsers, tc.regexParsers)
		if err != nil {
			t.Errorf("test %q got error: %v, want no error", tc.name, err)
			return
		}
		if diff := diff.Diff(tc.want, got); diff != "" {
			t.Errorf("test %q: ran GenerateFluentBitParserConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
		}
	}
}

func TestGenerateFluentBitParserConfigErrors(t *testing.T) {
	tests := []struct {
		name         string
		jsonParsers  []*ParserJSON
		regexParsers []*ParserRegex
	}{
		{
			name:        "an invalid json parser exists",
			jsonParsers: []*ParserJSON{{}},
		},
		{
			name:         "an invalid regex parser exists",
			regexParsers: []*ParserRegex{{}},
		},
	}
	for _, tc := range tests {
		if _, err := GenerateFluentBitParserConfig(tc.jsonParsers, tc.regexParsers); err == nil {
			t.Errorf("test %q: GenerateFluentBitParserConfig succeeded, want error", tc.name)
		}
	}
}
