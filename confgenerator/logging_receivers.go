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

type LoggingReceiverFiles struct {
	ConfigComponent `yaml:",inline"`

	IncludePaths []string `yaml:"include_paths,omitempty" validate:"required"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverFiles{} })
}

type LoggingReceiverSyslog struct {
	ConfigComponent `yaml:",inline"`

	TransportProtocol string `yaml:"transport_protocol,omitempty" validate:"oneof=tcp udp"`
	ListenHost        string `yaml:"listen_host,omitempty" validate:"required,ip"`
	ListenPort        uint16 `yaml:"listen_port,omitempty" validate:"required"`
}

func (r LoggingReceiverSyslog) Type() string {
	return "syslog"
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverSyslog{} })
}

type LoggingReceiverWinevtlog struct {
	ConfigComponent `yaml:",inline"`

	Channels []string `yaml:"channels,omitempty,flow" validate:"required"`
}

func (r LoggingReceiverWinevtlog) Type() string {
	return "windows_event_log"
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverWinevtlog{} })
}
