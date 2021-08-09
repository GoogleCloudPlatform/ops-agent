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
	"net"
	"strings"
)

type LoggingReceiverFiles struct {
	configComponent `yaml:",inline"`

	IncludePaths []string `yaml:"include_paths,omitempty" validate:"required"`
	ExcludePaths []string `yaml:"exclude_paths,omitempty"`
}

func (r LoggingReceiverFiles) Type() string {
	return "files"
}

func (r *LoggingReceiverFiles) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*r, subagent, kind, id, r.Type())
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverFiles{} })
}

type LoggingReceiverSyslog struct {
	configComponent `yaml:",inline"`

	TransportProtocol string `yaml:"transport_protocol,omitempty"` // one of "tcp" or "udp"
	ListenHost        string `yaml:"listen_host,omitempty" validate:"required"`
	ListenPort        uint16 `yaml:"listen_port,omitempty" validate:"required"`
}

func (r LoggingReceiverSyslog) Type() string {
	return "syslog"
}

func (r *LoggingReceiverSyslog) ValidateParameters(subagent string, kind string, id string) error {
	if err := validateParameters(*r, subagent, kind, id, r.Type()); err != nil {
		return err
	}
	validProtocolValues := []string{"tcp", "udp"}
	if !sliceContains(validProtocolValues, r.TransportProtocol) {
		err := fmt.Errorf(`must be one of [%s].`, strings.Join(validProtocolValues, ", "))
		return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, r.Type(), "transport_protocol"), r.TransportProtocol, err)
	}
	if net.ParseIP(r.ListenHost) == nil {
		err := fmt.Errorf(`must be a valid IP.`)
		return fmt.Errorf(`%s has invalid value %q: %s`, parameterErrorPrefix(subagent, kind, id, r.Type(), "listen_host"), r.ListenHost, err)
	}
	return nil
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverSyslog{} })
}

type LoggingReceiverWinevtlog struct {
	configComponent `yaml:",inline"`

	Channels []string `yaml:"channels,omitempty,flow" validate:"required"`
}

func (r LoggingReceiverWinevtlog) Type() string {
	return "windows_event_log"
}

func (r *LoggingReceiverWinevtlog) ValidateParameters(subagent string, kind string, id string) error {
	return validateParameters(*r, subagent, kind, id, r.Type())
}

func init() {
	loggingReceiverTypes.registerType(func() component { return &LoggingReceiverWinevtlog{} })
}
