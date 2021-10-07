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

import (
	"fmt"
)

func TranslationComponents(tag, src, dest string, translations []struct{ SrcVal, DestVal string }) []Component {
	c := []Component{}
	for _, t := range translations {
		c = append(c, Component{
			Kind: "FILTER",
			Config: map[string]string{
				"Name":      "modify",
				"Match":     tag,
				"Condition": fmt.Sprintf("Key_Value_Equals %s %s", src, t.SrcVal),
				"Add":       fmt.Sprintf("%s %s", dest, t.DestVal),
			},
		})
	}

	return c
}
