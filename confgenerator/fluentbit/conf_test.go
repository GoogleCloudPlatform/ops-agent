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
    Name     parser
    Match    test_match
    Key_Name test_key_name
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
    Name                  rewrite_tag
    Match                 test_match
    Rule                  $logName .* $logName false
    Emitter_Storage.type  filesystem
    Emitter_Mem_Buf_Limit 10M`
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
    Name        test_name
    Format      json
    Time_Key    test_time_key
    Time_Format test_time_format`,
		},
		{
			parserJSON: ParserJSON{
				Name:       "test_name",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name        test_name
    Format      json
    Time_Format test_time_format`,
		},
		{
			parserJSON: ParserJSON{
				Name:    "test_name",
				TimeKey: "test_time_key",
			},
			expectedTailConfig: `[PARSER]
    Name        test_name
    Format      json
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
    Name        test_name
    Format      regex
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
    Name        test_name
    Format      regex
    Regex       test_regex
    Time_Key    test_time_key
    Time_Format test_time_format`,
		},
		{
			parserRegex: ParserRegex{
				Name:       "test_name",
				Regex:      "test_regex",
				TimeFormat: "test_time_format",
			},
			expectedTailConfig: `[PARSER]
    Name        test_name
    Format      regex
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
    Name        test_name
    Format      regex
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
				Tag:  "test_tag",
				DB:   "test_db",
				Path: "test_path",
			},
			expectedTailConfig: `[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag
    Path               test_path
    DB                 test_db
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M`,
		},
		{
			tail: Tail{
				Tag:         "test_tag",
				DB:          "test_db",
				Path:        "test_path",
				ExcludePath: "test_exclude_path",
			},
			expectedTailConfig: `[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag
    Path               test_path
    DB                 test_db
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On
    # Exclude files matching this criteria.
    Exclude_Path       test_exclude_path

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M`,
		},
		{
			tail: Tail{
				Tag:         "test_tag",
				DB:          "test_db",
				Path:        "test_path",
				ExcludePath: "test_exclude_path/file1,test_excloud_path/file2",
			},
			expectedTailConfig: `[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag
    Path               test_path
    DB                 test_db
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On
    # Exclude files matching this criteria.
    Exclude_Path       test_exclude_path/file1,test_excloud_path/file2

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M`,
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
    # https://docs.fluentbit.io/manual/pipeline/inputs/syslog
    Name           syslog
    Tag            test_tag
    Mode           tcp
    Listen         0.0.0.0
    Port           1234
    Parser         lib:default_message_parser

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type   filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit  10M`,
		},
	}
	for _, tc := range tests {
		got, err := tc.syslog.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedSyslogConfig, got); diff != "" {
			t.Errorf("Tail %v: ran syslog.Generate() returned unexpected diff (-want +got):\n%s", tc.syslog, diff)
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
				DB:           "test_DB",
			},
			expectedWinlogConfig: `[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
    Name           winlog
    Tag            windows_event_log
    Channels       System,Application,Security
    Interval_Sec   1
    DB             test_DB`,
		},
	}
	for _, tc := range tests {
		got, err := tc.wineventlog.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.expectedWinlogConfig, got); diff != "" {
			t.Errorf("Tail %v: ran wineventlog.Generate() returned unexpected diff (-want +got):\n%s", tc.wineventlog, diff)
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
    # https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
    Name              stackdriver
    Match_Regex       ^(test_match)$
    resource          gce_instance
    stackdriver_agent user_agent
    workers           8

    # https://docs.fluentbit.io/manual/administration/scheduling-and-retries
    # After 3 retries, a given chunk will be discarded. So bad entries don't accidentally stay around forever.
    Retry_Limit  3

    # https://docs.fluentbit.io/manual/administration/security
    # Enable TLS support.
    tls         On
    # Do not force certificate validation.
    tls.verify  Off`
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
			want: `[SERVICE]
    # https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
    # Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
    Flush      1
    # We use systemd to manage Fluent Bit instead.
    Daemon     off
    # Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/logging-module.log).
    Log_Level  info

    # https://docs.fluentbit.io/manual/administration/monitoring
    # Enable a built-in HTTP server that can be used to query internal information and monitor metrics of each running plugin.
    HTTP_Server  On
    HTTP_Listen  0.0.0.0
    HTTP_PORT    2020

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
    # storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/fluent-bit/buffers).
    storage.sync               normal
    # Enable the data integrity check when writing and reading data from the filesystem.
    storage.checksum           on
    # The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
    storage.backlog.mem_limit  50M
    # Enable storage metrics in the built-in HTTP server.
    storage.metrics            on
    # This is exclusive to filesystem storage type. It specifies the number of chunks (every chunk is a file) that can be up in memory.
    # Every chunk is a file, so having it up in memory means having an open file descriptor. In case there are thousands of chunks,
    # we don't want them to all be loaded into the memory.
    storage.max_chunks_up      128`,
		},
		{
			name: "multiple tail and syslog plugins",
			inputs: []Input{
				&Tail{
					Tag:  "test_tag1",
					DB:   "test_db1",
					Path: "test_path1",
				}, &Tail{
					Tag:  "test_tag2",
					DB:   "test_db2",
					Path: "test_path2",
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
			want: `[SERVICE]
    # https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
    # Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
    Flush      1
    # We use systemd to manage Fluent Bit instead.
    Daemon     off
    # Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/logging-module.log).
    Log_Level  info

    # https://docs.fluentbit.io/manual/administration/monitoring
    # Enable a built-in HTTP server that can be used to query internal information and monitor metrics of each running plugin.
    HTTP_Server  On
    HTTP_Listen  0.0.0.0
    HTTP_PORT    2020

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
    # storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/fluent-bit/buffers).
    storage.sync               normal
    # Enable the data integrity check when writing and reading data from the filesystem.
    storage.checksum           on
    # The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
    storage.backlog.mem_limit  50M
    # Enable storage metrics in the built-in HTTP server.
    storage.metrics            on
    # This is exclusive to filesystem storage type. It specifies the number of chunks (every chunk is a file) that can be up in memory.
    # Every chunk is a file, so having it up in memory means having an open file descriptor. In case there are thousands of chunks,
    # we don't want them to all be loaded into the memory.
    storage.max_chunks_up      128

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag1
    Path               test_path1
    DB                 test_db1
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag2
    Path               test_path2
    DB                 test_db2
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/syslog
    Name           syslog
    Tag            test_tag1
    Mode           tcp
    Listen         0.0.0.0
    Port           1234
    Parser         lib:default_message_parser

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type   filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit  10M

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/syslog
    Name           syslog
    Tag            test_tag2
    Mode           udp
    Listen         0.0.0.0
    Port           5678
    Parser         lib:default_message_parser

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type   filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit  10M`,
		},
	}
	for _, tc := range tests {
		got, _, err := Config{
			Inputs: tc.inputs,
		}.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("test %q: ran GenerateFluentBitMainConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
		}
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
					Tag:  "test_tag1",
					DB:   "test_db1",
					Path: "test_path1",
				}, &Tail{
					Tag:  "test_tag2",
					DB:   "test_db2",
					Path: "test_path2",
				},
				&WindowsEventlog{
					Tag:          "win_tag1",
					Channels:     "chl1",
					Interval_Sec: "1",
					DB:           "test_DB1",
				}, &WindowsEventlog{
					Tag:          "win_tag2",
					Channels:     "chl2",
					Interval_Sec: "1",
					DB:           "test_DB2",
				},
			},
			want: `[SERVICE]
    # https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
    # Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
    Flush      1
    # We use systemd to manage Fluent Bit instead.
    Daemon     off
    # Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/logging-module.log).
    Log_Level  info

    # https://docs.fluentbit.io/manual/administration/monitoring
    # Enable a built-in HTTP server that can be used to query internal information and monitor metrics of each running plugin.
    HTTP_Server  On
    HTTP_Listen  0.0.0.0
    HTTP_PORT    2020

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
    # storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/fluent-bit/buffers).
    storage.sync               normal
    # Enable the data integrity check when writing and reading data from the filesystem.
    storage.checksum           on
    # The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
    storage.backlog.mem_limit  50M
    # Enable storage metrics in the built-in HTTP server.
    storage.metrics            on
    # This is exclusive to filesystem storage type. It specifies the number of chunks (every chunk is a file) that can be up in memory.
    # Every chunk is a file, so having it up in memory means having an open file descriptor. In case there are thousands of chunks,
    # we don't want them to all be loaded into the memory.
    storage.max_chunks_up      128

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag1
    Path               test_path1
    DB                 test_db1
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/tail#config
    Name               tail
    Tag                test_tag2
    Path               test_path2
    DB                 test_db2
    Read_from_Head     True
    # Set the chunk limit conservatively to avoid exceeding the recommended chunk size of 5MB per write request.
    Buffer_Chunk_Size  512k
    # Set the max size a bit larger to accommodate for long log lines.
    Buffer_Max_Size    5M
    # When a message is unstructured (no parser applied), append it under a key named "message".
    Key                message
    # Increase this to 30 seconds so log rotations are handled more gracefully.
    Rotate_Wait        30
    # Skip long lines instead of skipping the entire file when a long line exceeds buffer size.
    Skip_Long_Lines    On

    # https://docs.fluentbit.io/manual/administration/buffering-and-storage#input-section-configuration
    # Buffer in disk to improve reliability.
    storage.type       filesystem

    # https://docs.fluentbit.io/manual/administration/backpressure#mem_buf_limit
    # This controls how much data the input plugin can hold in memory once the data is ingested into the core.
    # This is used to deal with backpressure scenarios (e.g: cannot flush data for some reason).
    # When the input plugin hits "mem_buf_limit", because we have enabled filesystem storage type, mem_buf_limit acts
    # as a hint to set "how much data can be up in memory", once the limit is reached it continues writing to disk.
    Mem_Buf_Limit      10M

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
    Name           winlog
    Tag            win_tag1
    Channels       chl1
    Interval_Sec   1
    DB             test_DB1

[INPUT]
    # https://docs.fluentbit.io/manual/pipeline/inputs/windows-event-log
    Name           winlog
    Tag            win_tag2
    Channels       chl2
    Interval_Sec   1
    DB             test_DB2`,
		},
	}
	for _, tc := range tests {
		got, _, err := Config{
			Inputs: tc.inputs,
		}.Generate()
		if err != nil {
			t.Errorf("got error: %v, want no error", err)
			return
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("test %q: ran GenerateFluentBitMainConfig returned unexpected diff (-want +got):\n%s", tc.name, diff)
		}
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
    Name        lib:default_message_parser
    Format      regex
    Regex       ^(?<message>.*)$

[PARSER]
    Name        lib:apache
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:apache2
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name   lib:apache_error
    Format regex
    Regex  ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Name        lib:mongodb
    Format      regex
    Regex       ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L

[PARSER]
    Name        lib:nginx
    Format      regex
    Regex       ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%Z

[PARSER]
    Name        lib:syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

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
    Name        lib:default_message_parser
    Format      regex
    Regex       ^(?<message>.*)$

[PARSER]
    Name        lib:apache
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:apache2
    Format      regex
    Regex       ^(?<host>[^ ]*) [^ ]* (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^ ]*) +\S*)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>.*)")?$
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name   lib:apache_error
    Format regex
    Regex  ^\[[^ ]* (?<time>[^\]]*)\] \[(?<level>[^\]]*)\](?: \[pid (?<pid>[^\]]*)\])?( \[client (?<client>[^\]]*)\])? (?<message>.*)$

[PARSER]
    Name        lib:mongodb
    Format      regex
    Regex       ^(?<time>[^ ]*)\s+(?<severity>\w)\s+(?<component>[^ ]+)\s+\[(?<context>[^\]]+)]\s+(?<message>.*?) *(?<ms>(\d+))?(:?ms)?$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L

[PARSER]
    Name        lib:nginx
    Format      regex
    Regex       ^(?<remote>[^ ]*) (?<host>[^ ]*) (?<user>[^ ]*) \[(?<time>[^\]]*)\] "(?<method>\S+)(?: +(?<path>[^\"]*?)(?: +\S*)?)?" (?<code>[^ ]*) (?<size>[^ ]*)(?: "(?<referer>[^\"]*)" "(?<agent>[^\"]*)")
    Time_Key    time
    Time_Format %d/%b/%Y:%H:%M:%S %z

[PARSER]
    Name        lib:syslog-rfc5424
    Format      regex
    Regex       ^\<(?<pri>[0-9]{1,5})\>1 (?<time>[^ ]+) (?<host>[^ ]+) (?<ident>[^ ]+) (?<pid>[-0-9]+) (?<msgid>[^ ]+) (?<extradata>(\[(.*?)\]|-)) (?<message>.+)$
    Time_Key    time
    Time_Format %Y-%m-%dT%H:%M:%S.%L%Z

[PARSER]
    Name        lib:syslog-rfc3164
    Format      regex
    Regex       /^\<(?<pri>[0-9]+)\>(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$/
    Time_Key    time
    Time_Format %b %d %H:%M:%S

[PARSER]
    Name        test_name1
    Format      json
    Time_Key    test_time_key1
    Time_Format test_time_format1

[PARSER]
    Name        test_name2
    Format      json
    Time_Key    test_time_key2
    Time_Format test_time_format2

[PARSER]
    Name        test_name1
    Format      regex
    Regex       test_regex1

[PARSER]
    Name        test_name2
    Format      regex
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
