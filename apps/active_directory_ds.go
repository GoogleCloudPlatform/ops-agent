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

type LoggingReceiverActiveDirectoryDS struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (r LoggingReceiverActiveDirectoryDS) Type() string {
	return "active_directory_ds"
}

func (r LoggingReceiverActiveDirectoryDS) Components(tag string) []fluentbit.Component {
	l := confgenerator.LoggingReceiverWindowsEventLog{
		Channels: []string{"Directory Service", "Active Directory Web Services"},
	}

	return l.Components(tag)
}

func init() {
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverActiveDirectoryDS{} }, "windows")
}
