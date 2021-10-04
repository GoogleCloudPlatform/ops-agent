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

package apps

import (
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorRedis struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorRedis) Type() string {
	return "redis"
}

func (p LoggingProcessorRedis) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Documentation: https://github.com/redis/redis/blob/6.2/src/server.c#L1122
		// Sample line: 534:M 28 Apr 2020 11:30:29.988 * DB loaded from disk: 0.002 seconds
		// Sample older format line: [4018] 14 Nov 07:01:22.119 * Background saving terminated with success
		Regex: `^\[?(?<pid>\d+):?(?<roleChar>[A-Z])?\]?\s+(?<time>\d{2}\s+\w+(?:\s+\d{4})?\s+\d{2}:\d{2}:\d{2}.\d{3})\s+(?<level>(\*|#|-|\.))\s+(?<message>.*)$`,
		LoggingProcessorParseShared: confgenerator.LoggingProcessorParseShared{
			TimeKey:    "time",
			TimeFormat: "%d %b %Y %H:%M:%S.%L",
			Types: map[string]string{
				"pid": "integer",
			},
		},
	}.Components(tag, uid)

	// Log levels documented: https://github.com/redis/redis/blob/6.2/src/server.c#L1124
	c = append(c,
		TranslationComponents(tag, "level", "logging.googleapis.com/severity",
			[]struct{ srcVal, destVal string }{
				{".", "DEBUG"},
				{"-", "INFO"},
				{"*", "NOTICE"},
				{"#", "WARNING"},
			},
		)...,
	)

	// Role translation documented: https://github.com/redis/redis/blob/6.2/src/server.c#L1149
	c = append(c,
		TranslationComponents(tag, "roleChar", "role",
			[]struct{ srcVal, destVal string }{
				{"X", "sentinel"},
				{"C", "RDB/AOF_writing_child"},
				{"S", "slave"},
				{"M", "master"},
			},
		)...,
	)

	return c
}

type LoggingReceiverRedis struct {
	LoggingProcessorRedis                   `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverRedis) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			// Default log path on Ubuntu / Debian
			"/var/log/redis/redis-server.log",
			// Default log path built from src
			"/var/log/redis_6379.log",
			// Default log path on CentOS / RHEL
			"/var/log/redis/redis.log",
			// Default log path on SLES
			"/var/log/redis/default.log",
			// Default log path from one click installer
			"/var/log/redis/redis_6379.log",
		}
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorRedis.Components(tag, "redis")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorRedis{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverRedis{} })
}
