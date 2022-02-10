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

type LoggingProcessorRabbitmq struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (*LoggingProcessorRabbitmq) Type() string {
	return "rabbitmq"
}

func (p *LoggingProcessorRabbitmq) Components(tag, uid string) []fluentbit.Component {
	c := []fluentbit.Component{}
	regexParser := confgenerator.LoggingProcessorParseRegex{
		// Sample log line:
		// 2022-01-31 18:01:20.441571+00:00 [erro] <0.692.0> ** Connection attempt from node 'rabbit_ctl_17@keith-testing-rabbitmq' rejected. Invalid challenge reply. **
		Regex: `^(?<timestamp>\d+-\d+-\d+ \d+:\d+:\d+\.\d+\+\d+:\d+) \[(?<severity>\w+)\] \<(?<process_id>\d+\.\d+\.\d+)\>(?<message>.*)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "timestamp",
			TimeFormat: "%Y-%m-%d %H:%M:%S.%L+%Z",
		},
	}
	c = append(c, regexParser.Components(tag, uid)...)

	// severities documented here: https://www.rabbitmq.com/logging.html#log-levels
	c = append(c, fluentbit.TranslationComponents(tag, "severity", "logging.googleapis.com/severity", true, []struct {
		SrcVal  string
		DestVal string
	}{
		{
			"debug", "DEBUG",
		},
		{
			"erro", "ERROR",
		},
		{
			"info", "INFO",
		},
		{
			"noti", "DEFAULT",
		},
	})...)

	return c
}

type LoggingReceiverRabbitmq struct {
	LoggingProcessorRabbitmq                `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverRabbitmq) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/var/log/rabbitmq/rabbit*.log",
		}
	}
	// Some multiline entries related to crash logs are important to capture and end in
	//
	// 2022-01-31 18:07:43.557042+00:00 [erro] <0.130.0>
	// BOOT FAILED
	// ===========
	// ERROR: could not bind to distribution port 25672, it is in use by another node: rabbit@keith-testing-rabbitmq
	//
	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^(?<timestamp>\d+-\d+-\d+ \d+:\d+:\d+\.\d+\+\d+:\d+) \[(?<severity>\w+)\] \<(?<process_id>\d+\.\d+\.\d+)\>(?<message>.*)$`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^\s*$`,
		},
	}
	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorRabbitmq.Components(tag, "rabbitmq")...)
	return c
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverRabbitmq{} })
}
