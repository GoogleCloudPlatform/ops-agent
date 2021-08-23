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

import "fmt"

// TODO: Move structs out of conf.go

func (f FilterParser) Component() Component {
	return Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":     "parser",
			"Match":    f.Match,
			"Key_Name": f.KeyName,
			"Parser":   f.Parser,
		},
	}
}

func (p ParserJSON) Component() Component {
	return Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Format":      "json",
			"Name":        p.Name,
			"Time_Format": p.TimeFormat,
			"Time_Key":    p.TimeKey,
		},
	}
}

func (p ParserRegex) Component() Component {
	return Component{
		Kind: "PARSER",
		Config: map[string]string{
			"Format":      "regex",
			"Name":        p.Name,
			"Regex":       p.Regex,
			"Time_Format": p.TimeFormat,
			"Time_Key":    p.TimeKey,
		},
	}
}

func (f FilterModifyAddLogName) Component() Component {
	return Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":  "modify",
			"Match": f.Match,
			"Add":   fmt.Sprintf("logName %s", f.LogName),
		},
	}
}

func (f FilterModifyRemoveLogName) Component() Component {
	return Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":   "modify",
			"Match":  f.Match,
			"Remove": "logName",
		},
	}
}

func (f FilterRewriteTag) Component() Component {
	return Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":                  "rewrite_tag",
			"Match":                 f.Match,
			"Rule":                  "$logName .* $logName false",
			"Emitter_Mem_Buf_Limit": "10M",
			"Emitter_Storage.type":  "filesystem",
		},
	}
}
