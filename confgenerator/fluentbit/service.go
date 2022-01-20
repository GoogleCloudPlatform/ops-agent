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

type Service struct {
	// Allowed log levels are: error, warn, info, debug, and trace.
	LogLevel string
}

func (s Service) Component() Component {
	return Component{
		Kind: "SERVICE",
		Config: map[string]string{
			// https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/configuration-file#config_section
			// Flush logs every 1 second, even if the buffer is not full to minimize log entry arrival delay.
			"Flush": "1",
			// We use systemd to manage Fluent Bit instead.
			"Daemon": "off",
			// Log_File is set by Fluent Bit systemd unit (e.g. /var/log/google-cloud-ops-agent/subagents/logging-module.log).
			"Log_Level": s.LogLevel,

			// Use the legacy DNS resolver mechanism to work around b/206549605 temporarily.
			"dns.resolver": "legacy",
			// https://docs.fluentbit.io/manual/administration/buffering-and-storage#service-section-configuration
			// storage.path is set by Fluent Bit systemd unit (e.g. /var/lib/google-cloud-ops-agent/fluent-bit/buffers).
			"storage.sync": "normal",
			// Enable the data integrity check when writing and reading data from the filesystem.
			"storage.checksum": "on",
			// The maximum amount of data to load into the memory when processing old chunks from the backlog that is from previous Fluent Bit processes (e.g. Fluent Bit may have crashed or restarted).
			"storage.backlog.mem_limit": "50M",
			// Enable storage metrics in the built-in HTTP server.
			"storage.metrics": "on",
			// This is exclusive to filesystem storage type. It specifies the number of chunks (every chunk is a file) that can be up in memory.
			// Every chunk is a file, so having it up in memory means having an open file descriptor. In case there are thousands of chunks,
			// we don't want them to all be loaded into the memory.
			"storage.max_chunks_up": "128",
		},
	}
}

func (s Service) MetricsComponent() Component {
	return Component{
		Kind: "INPUT",
		Config: map[string]string{
			"Name":            "fluentbit_metrics",
			"Scrape_On_Start": "True",
			"Scrape_Interval": "60",
		},
	}
}
