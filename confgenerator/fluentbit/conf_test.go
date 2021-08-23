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

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFilterParser(t *testing.T) {
	f := FilterParser{
		Match:   "test_match",
		KeyName: "test_key_name",
		Parser:  "test_parser",
	}
	want := `[FILTER]
    Key_Name test_key_name
    Match    test_match
    Name     parser
    Parser   test_parser`
	got, err := f.Generate()
	if err != nil {
		t.Errorf("got error: %v, want no error", err)
		return
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FilterParser %v: FilterParser.Generate() returned unexpected diff (-want +got):\n%s", want, diff)
	}
}

func TestFilterRewriteTag(t *testing.T) {
	f := FilterRewriteTag{
		Match: "test_match",
	}
	want := `[FILTER]
    Emitter_Mem_Buf_Limit 10M
    Emitter_Storage.type  filesystem
    Match                 test_match
    Name                  rewrite_tag
    Rule                  $logName .* $logName false`
	got, err := f.Generate()
	if err != nil {
		t.Errorf("got error: %v, want no error", err)
		return
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FilterRewriteTag %v: FilterRewriteTag.Generate() returned unexpected diff (-want +got):\n%s", want, diff)
	}
}

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
    Format      json
    Name        test_name
    Time_Format test_time_format
    Time_Key    test_time_key`,
		},
		{
			parserJSON: ParserJSON{
				Name:       "test_name",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Format      json
    Name        test_name
    Time_Format test_time_format`,
		},
		{
			parserJSON: ParserJSON{
				Name:    "test_name",
				TimeKey: "test_time_key",
			},
			expectedTailConfig: `[PARSER]
    Format      json
    Name        test_name
    Time_Key    test_time_key`,
		},
	}
	for _, tc := range tests {
		got, err := tc.parserJSON.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("ParserJSON %v: ParserJSON.Generate() returned unexpected diff (-want +got):\n%s", tc.parserJSON, diff)
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
    Format      regex
    Name        test_name
    Regex       test_regex`,
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				Regex:      "test_regex",
				TimeKey:    "test_time_key",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Format      regex
    Name        test_name
    Regex       test_regex
    Time_Format test_time_format
    Time_Key    test_time_key`,
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				Regex:      "test_regex",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Format      regex
    Name        test_name
    Regex       test_regex
    Time_Format test_time_format`,
		},
		{
			parserRegex: ParserRegex{
				Name:    "test_name",
				Regex:   "test_regex",
				TimeKey: "test_time_key",
			},
			expectedTailConfig: `[PARSER]
    Format      regex
    Name        test_name
    Regex       test_regex
    Time_Key    test_time_key`,
		},
	}
	for _, tc := range tests {
		got, err := tc.parserRegex.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return

		}
		if diff := cmp.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("ParserRegex %v: ParserRegex.Generate() returned unexpected diff (-want +got):\n%s", tc.parserRegex, diff)
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
				Tag:          "test_tag",
				IncludePaths: []string{"test_path", "test_path_2"},
			},
			expectedTailConfig: `[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path,test_path_2
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag
    storage.type      filesystem`,
		},
		{
			tail: Tail{
				Tag:          "test_tag",
				IncludePaths: []string{"test_path"},
				ExcludePaths: []string{"test_exclude_path"},
			},
			expectedTailConfig: `[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag
    Exclude_Path      test_exclude_path
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag
    storage.type      filesystem`,
		},
		{
			tail: Tail{
				Tag:          "test_tag",
				IncludePaths: []string{"test_path"},
				ExcludePaths: []string{"test_exclude_path/file1", "test_exclude_path/file2"},
			},
			expectedTailConfig: `[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag
    Exclude_Path      test_exclude_path/file1,test_exclude_path/file2
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag
    storage.type      filesystem`,
		},
	}
	for _, tc := range tests {
		got, err := tc.tail.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedTailConfig, got); diff != "" {
			t.Errorf("Tail %v: ran Tail.Generate() returned unexpected diff (-want +got):\n%s", tc.tail, diff)
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
				Tag:    "test_tag",
			},
			expectedSyslogConfig: `[INPUT]
    Listen        0.0.0.0
    Mem_Buf_Limit 10M
    Mode          tcp
    Name          syslog
    Parser        lib:default_message_parser
    Port          1234
    Tag           test_tag
    storage.type  filesystem`,
		},
	}
	for _, tc := range tests {
		got, err := tc.syslog.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedSyslogConfig, got); diff != "" {
			t.Errorf("Syslog %v: ran syslog.Generate() returned unexpected diff (-want +got):\n%s", tc.syslog, diff)
		}
	}
}

func TestWinlog(t *testing.T) {
	tests := []struct {
		wineventlog          WindowsEventlog
		expectedWinlogConfig string
	}{
		{
			wineventlog: WindowsEventlog{
				Tag:          "windows_event_log",
				Channels:     "System,Application,Security",
				Interval_Sec: "1",
			},
			expectedWinlogConfig: `[INPUT]
    Channels     System,Application,Security
    DB           ${buffers_dir}/windows_event_log
    Interval_Sec 1
    Name         winlog
    Tag          windows_event_log`,
		},
	}
	for _, tc := range tests {
		got, err := tc.wineventlog.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedWinlogConfig, got); diff != "" {
			t.Errorf("WindowsEventlog %v: ran wineventlog.Generate() returned unexpected diff (-want +got):\n%s", tc.wineventlog, diff)
		}
	}
}

func TestStackdriver(t *testing.T) {
	s := Stackdriver{
		Match:     "test_match",
		UserAgent: "user_agent",
		Workers:   8,
	}
	want := `[OUTPUT]
    Match_Regex       ^(test_match)$
    Name              stackdriver
    Retry_Limit       3
    resource          gce_instance
    stackdriver_agent user_agent
    tls               On
    tls.verify        Off
    workers           8`
	got, err := s.Generate()
	if err != nil {
		t.Errorf("got error: %v, want no error", err)
		return
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Stackdriver %v: Stackdriver.Generate() returned unexpected diff (-want +got):\n%s", want, diff)
	}
}

func TestGenerateFluentBitMainConfig(t *testing.T) {
	tests := []struct {
		name   string
		inputs []Input
		want   string
	}{
		{
			name: "zero plugins",
			want: `@SET buffers_dir=/state/buffers
@SET logs_dir=/logs

[SERVICE]
    Daemon                    off
    Flush                     1
    HTTP_Listen               0.0.0.0
    HTTP_PORT                 2020
    HTTP_Server               On
    Log_Level                 info
    storage.backlog.mem_limit 50M
    storage.checksum          on
    storage.max_chunks_up     128
    storage.metrics           on
    storage.sync              normal`,
		},
		{
			name: "multiple tail and syslog plugins",
			inputs: []Input{
				&Tail{
					Tag:          "test_tag1",
					IncludePaths: []string{"test_path1"},
				}, &Tail{
					Tag:          "test_tag2",
					IncludePaths: []string{"test_path2"},
				},
				&Syslog{
					Mode:   "tcp",
					Listen: "0.0.0.0",
					Port:   1234,
					Tag:    "test_tag1",
				}, &Syslog{
					Mode:   "udp",
					Listen: "0.0.0.0",
					Port:   5678,
					Tag:    "test_tag2",
				},
			},
			want: `@SET buffers_dir=/state/buffers
@SET logs_dir=/logs

[SERVICE]
    Daemon                    off
    Flush                     1
    HTTP_Listen               0.0.0.0
    HTTP_PORT                 2020
    HTTP_Server               On
    Log_Level                 info
    storage.backlog.mem_limit 50M
    storage.checksum          on
    storage.max_chunks_up     128
    storage.metrics           on
    storage.sync              normal

[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag1
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path1
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag1
    storage.type      filesystem

[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag2
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path2
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag2
    storage.type      filesystem

[INPUT]
    Listen        0.0.0.0
    Mem_Buf_Limit 10M
    Mode          tcp
    Name          syslog
    Parser        lib:default_message_parser
    Port          1234
    Tag           test_tag1
    storage.type  filesystem

[INPUT]
    Listen        0.0.0.0
    Mem_Buf_Limit 10M
    Mode          udp
    Name          syslog
    Parser        lib:default_message_parser
    Port          5678
    Tag           test_tag2
    storage.type  filesystem`,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := Config{
				StateDir: "/state",
				LogsDir:  "/logs",
				Inputs:   tc.inputs,
			}.Generate()
			if err != nil {
				t.Errorf("got error: %v, want no error", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ran GenerateFluentBitMainConfig returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateFluentBitMainConfigWindows(t *testing.T) {
	tests := []struct {
		name   string
		inputs []Input
		want   string
	}{
		{
			name: "multiple tail and winlog plugins",
			inputs: []Input{
				&Tail{
					Tag:          "test_tag1",
					IncludePaths: []string{"test_path1"},
				}, &Tail{
					Tag:          "test_tag2",
					IncludePaths: []string{"test_path2"},
				},
				&WindowsEventlog{
					Tag:          "win_tag1",
					Channels:     "chl1",
					Interval_Sec: "1",
				}, &WindowsEventlog{
					Tag:          "win_tag2",
					Channels:     "chl2",
					Interval_Sec: "1",
				},
			},
			want: `@SET buffers_dir=/state/buffers
@SET logs_dir=/logs

[SERVICE]
    Daemon                    off
    Flush                     1
    HTTP_Listen               0.0.0.0
    HTTP_PORT                 2020
    HTTP_Server               On
    Log_Level                 info
    storage.backlog.mem_limit 50M
    storage.checksum          on
    storage.max_chunks_up     128
    storage.metrics           on
    storage.sync              normal

[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag1
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path1
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag1
    storage.type      filesystem

[INPUT]
    Buffer_Chunk_Size 512k
    Buffer_Max_Size   5M
    DB                ${buffers_dir}/test_tag2
    Key               message
    Mem_Buf_Limit     10M
    Name              tail
    Path              test_path2
    Read_from_Head    True
    Rotate_Wait       30
    Skip_Long_Lines   On
    Tag               test_tag2
    storage.type      filesystem

[INPUT]
    Channels     chl1
    DB           ${buffers_dir}/win_tag1
    Interval_Sec 1
    Name         winlog
    Tag          win_tag1

[INPUT]
    Channels     chl2
    DB           ${buffers_dir}/win_tag2
    Interval_Sec 1
    Name         winlog
    Tag          win_tag2`,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := Config{
				StateDir: "/state",
				LogsDir:  "/logs",
				Inputs:   tc.inputs,
			}.Generate()
			if err != nil {
				t.Errorf("got error: %v, want no error", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ran GenerateFluentBitMainConfig returned unexpected diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateFluentBitParserConfig(t *testing.T) {
	tests := []struct {
		name    string
		parsers []Parser
		want    string
	}{
		{
			name: "empty JSON Parsers and Regex Parsers",
			want: `[PARSER]
    Format      regex
    Name        lib:default_message_parser
    Regex       ^(?<message>.*)$

[PARSER]
    Format      regex
    Name        lib:apache
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:apache2
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:apache_error
    Regex       ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Format      regex
    Name        lib:mongodb
    Regex       ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Format %Y-%m-%dT%H:%M:%S.%L
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:nginx
    Regex       ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:syslog-rfc5424
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Format %Y-%m-%dT%H:%M:%S.%L%Z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:syslog-rfc3164
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Format %b %d %H:%M:%S
    Time_Key    time

`,
		},
		{
			name: "multiple JSON Parsers and Regex Parsers",
			parsers: []Parser{
				&ParserJSON{
					Name:       "test_name1",
					TimeKey:    "test_time_key1",
					TimeFormat: "test_time_format1",
				}, &ParserJSON{
					Name:       "test_name2",
					TimeKey:    "test_time_key2",
					TimeFormat: "test_time_format2",
				},
				&ParserRegex{
					Name:  "test_name1",
					Regex: "test_regex1",
				}, &ParserRegex{
					Name:  "test_name2",
					Regex: "test_regex2",
				},
			},
			want: `[PARSER]
    Format      regex
    Name        lib:default_message_parser
    Regex       ^(?<message>.*)$

[PARSER]
    Format      regex
    Name        lib:apache
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:apache2
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:apache_error
    Regex       ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Format      regex
    Name        lib:mongodb
    Regex       ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Format %Y-%m-%dT%H:%M:%S.%L
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:nginx
    Regex       ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Format %d/%b/%Y:%H:%M:%S %z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:syslog-rfc5424
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Format %Y-%m-%dT%H:%M:%S.%L%Z
    Time_Key    time

[PARSER]
    Format      regex
    Name        lib:syslog-rfc3164
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Format %b %d %H:%M:%S
    Time_Key    time

[PARSER]
    Format      json
    Name        test_name1
    Time_Format test_time_format1
    Time_Key    test_time_key1

[PARSER]
    Format      json
    Name        test_name2
    Time_Format test_time_format2
    Time_Key    test_time_key2

[PARSER]
    Format      regex
    Name        test_name1
    Regex       test_regex1

[PARSER]
    Format      regex
    Name        test_name2
    Regex       test_regex2

`,
		},
	}
	for _, tc := range tests {
		_, got, err := Config{
			Parsers: tc.parsers,
		}.Generate()
		if err != nil {
			t.Errorf("test %q got error: %v, want no error", tc.name, err)
			return
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("test %q: ran GenerateFluentBitParserConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
		}
	}
}
