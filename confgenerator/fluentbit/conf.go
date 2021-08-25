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

// Package fluentbit provides data structures to represent and generate fluentBit configuration.
package fluentbit

// A Tail represents the configuration data for fluentBit's tail input plugin.
type Tail struct {
	Tag          string
	IncludePaths []string
	ExcludePaths []string
}

// A Syslog represents the configuration data for fluentBit's syslog input plugin.
type Syslog struct {
	Tag    string
	Mode   string
	Listen string
	Port   uint16
}

// A WindowsEventlog represents the configuration data for fluentbit's winlog input plugin
type WindowsEventlog struct {
	Tag          string
	Channels     []string
	Interval_Sec string // XXX: stop ignoring?
}

// A Stackdriver represents the configurable data for fluentBit's stackdriver output plugin.
type Stackdriver struct {
	Match     string
	UserAgent string
	Workers   int
}
