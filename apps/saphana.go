// Copyright 2022 Google LLC
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
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type LoggingProcessorSapHanaTrace struct {
	confgenerator.ConfigComponent `yaml:",inline"`
}

func (LoggingProcessorSapHanaTrace) Type() string {
	return "saphana_trace"
}

func (p LoggingProcessorSapHanaTrace) Components(tag string, uid string) []fluentbit.Component {
	c := confgenerator.LoggingProcessorParseRegex{
		// Undocumented Format: [thread_id]{connection_id}[transaction_id/update_transaction_id] timestamp severity_flag component source_file : message
		// Sample line: [7893]{200068}[20/40637286] 2021-11-04 13:13:25.025767 w FileIO           FileSystem.cpp(00085) : Unsupported file system "ext4" for "/usr/sap/MMM/SYS/global/hdb/data/mnt00001"
		// Sample line: [18048]{-1}[-1/-1] 2020-11-10 12:24:23.424024 i Crypto           RootKeyStoreAccessor.cpp(00818) : Created new root key /usr/sap/MMM/SYS/global/hdb/security/ssfs:HDB_SERVER/3/PERSISTENCE
		// Sample line: [18048]{-1}[-1/-1] 2020-11-10 12:24:20.988943 e commlib          commlibImpl.cpp(00986) : ERROR: comm::connect to Host: 127.0.0.1, port: 30001, Error: exception  1: no.2110017  (Basis/IO/Stream/impl/NetworkChannel.cpp:2989)
		// 				System error: SO_ERROR has pending error for socket. rc=111: Connection refused. channel={<NetworkChannel>={<NetworkChannelBase>={this=139745749931736, fd=21, refCnt=1, local=127.0.0.1/58654_tcp, remote=127.0.0.1/30001_tcp, state=ConnectWait, pending=[----]}}}
		// 				exception throw location:
		// 				 1: 0x00007f1937d9095c in .LTHUNK27.lto_priv.2256+0x558 (libhdbbasis.so)
		// 				 ...
		//				 25: 0x0000563f44888831 in _GLOBAL__sub_I_setServiceStarting.cpp.lto_priv.239+0x520 (hdbnsutil)
		Regex: `^\[(?<thread_id>\d+)\]\{(?<connection_id>-?\d+)\}\[(?<transaction_id>-?\d+)\/(?<update_transaction_id>-?\d+)\]\s+(?<time>\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3,6}\d+)\s+(?<severity_flag>\w+)\s+(?<component>\w+)\s+(?<source_file>\S+)\s+:\s+(?<message>[\s\S]+)`,
		ParserShared: confgenerator.ParserShared{
			TimeKey:    "time",
			TimeFormat: "%Y-%m-%d %H:%M:%S.%L",
			Types: map[string]string{
				"thread_id":             "int",
				"connection_id":         "int",
				"transaction_id":        "int",
				"update_transaction_id": "int",
			},
		},
	}.Components(tag, uid)

	c = append(c,
		fluentbit.TranslationComponents(tag, "severity_flag", "logging.googleapis.com/severity", true,
			[]struct{ SrcVal, DestVal string }{
				{"d", "DEBUG"},
				{"i", "INFO"},
				{"w", "WARNING"},
				{"e", "ERROR"},
				{"f", "ALERT"},
			},
		)...,
	)

	// If a log is not associated with a connection/transaction the related
	// fields will be "-1", and we do not want to report those fields
	for _, field := range []string{
		"connection_id",
		"transaction_id",
		"update_transaction_id",
	} {
		c = append(c, fluentbit.Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s -1", field),
				"Remove":    field,
			},
		})
	}

	return c
}

type LoggingReceiverSapHanaTrace struct {
	LoggingProcessorSapHanaTrace            `yaml:",inline"`
	confgenerator.LoggingReceiverFilesMixin `yaml:",inline" validate:"structonly"`
}

func (r LoggingReceiverSapHanaTrace) Components(tag string) []fluentbit.Component {
	if len(r.IncludePaths) == 0 {
		r.IncludePaths = []string{
			"/usr/sap/*/HDB*/${HOSTNAME}/trace/*.trc",
		}
	}
	if len(r.ExcludePaths) == 0 {
		r.ExcludePaths = []string{
			"/usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver_history*.trc",
			"/usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver*loads*.trc",
			"/usr/sap/*/HDB*/${HOSTNAME}/trace/nameserver*executed_statements*.trc",
		}
	}

	r.MultilineRules = []confgenerator.MultilineRule{
		{
			StateName: "start_state",
			NextState: "cont",
			Regex:     `^\[\d+\]\{-?\d+\}`,
		},
		{
			StateName: "cont",
			NextState: "cont",
			Regex:     `^(?!\[\d+\]\{-?\d+\})`,
		},
	}

	c := r.LoggingReceiverFilesMixin.Components(tag)
	c = append(c, r.LoggingProcessorSapHanaTrace.Components(tag, "saphana_trace")...)
	return c
}

func init() {
	confgenerator.LoggingProcessorTypes.RegisterType(func() confgenerator.Component { return &LoggingProcessorSapHanaTrace{} })
	confgenerator.LoggingReceiverTypes.RegisterType(func() confgenerator.Component { return &LoggingReceiverSapHanaTrace{} })
}
