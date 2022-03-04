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
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit/modify"
)

type ModifyField struct {
	// Source of value for this field
	// XXX: Validate field names
	MoveFrom    string  `yaml:"move_from" validate:"required_without_all=CopyFrom StaticValue,excluded_with=CopyFrom StaticValue"`
	CopyFrom    string  `yaml:"copy_from" validate:"required_without_all=MoveFrom StaticValue,excluded_with=MoveFrom StaticValue"`
	StaticValue *string `yaml:"static_value" validate:"required_without_all=MoveFrom CopyFrom,excluded_with=MoveFrom CopyFrom"`

	// Name of field with copied value
	sourceField string `yaml:"-"`

	// Operations to perform
	MapValues map[string]string `yaml:"map_values"`
	// XXX: Decide what the type names should be
	Type   string `yaml:"type" validate:"omitempty,oneof=integer float"`
	OmitIf string `yaml:"omit_if" validate:"omitempty,filter"`
}

type LoggingProcessorModifyFields struct {
	ConfigComponent `yaml:",inline"`
	Fields          map[string]ModifyField `yaml:"fields"`
}

func (p LoggingProcessorModifyFields) Type() string {
	return "modify_fields"
}

func (p LoggingProcessorModifyFields) Components(tag, uid string) []fluentbit.Component {
	c, err := p.components(tag, uid)
	if err != nil {
		// It shouldn't be possible to get here if the input validation is working, so treat this as a code bug.
		panic(err)
	}
	return c
}
func (p LoggingProcessorModifyFields) components(tag, uid string) ([]fluentbit.Component, error) {
	var lua strings.Builder
	lua.WriteString(`
function process(tag, timestamp, record)
`)
	var components []fluentbit.Component
	// Step 1: Obtain any source values needed for move or copy
	var i int
	fieldMappings := map[string]string{}
	moveFromFields := map[string]bool{}
	for _, field := range p.Fields {
		for j, name := range []*string{&field.MoveFrom, &field.CopyFrom} {
			if *name == "" {
				continue
			}
			m, err := filter.NewMember(*name)
			if err != nil {
				return nil, fmt.Errorf("failed to parse field %q: %w", *name, err)
			}
			key, err := m.RecordAccessor()
			if err != nil {
				return nil, fmt.Errorf("failed to convert field %q to record accessor: %w", *name, err)
			}
			if _, ok := fieldMappings[key]; !ok {
				new := fmt.Sprintf("__field_%d", i)
				fieldMappings[key] = new
				i++
				// TODO: Do this with Lua for performance?
				components = append(components, modify.ModifyOptions{modify.CopyModifyKey, fmt.Sprintf("%s %s", new, key)}.Component(tag))
			}
			*name = fieldMappings[key]
			if j == 0 {
				ra, err := m.LuaAccessor(true)
				if err != nil {
					return nil, fmt.Errorf("failed to convert %v to Lua accessor: %w", m, err)
				}
				moveFromFields[ra] = true
			}
		}
	}
	// Step 2: Remove any MoveFrom fields
	for ra := range moveFromFields {
		fmt.Fprintf(&lua, `%s(nil)
`, ra)
	}
	// Step 3: Evaluate any OmitIf conditions
	// XXX
	// Step 4: Assign values
	for outName, field := range p.Fields {
		outM, err := filter.NewMember(outName)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output field %q: %m", outName, err)
		}

		// XXX: Process MapValues
		// XXX: Process Type

		src := "nil"
		if field.sourceField != "" {
			src = fmt.Sprintf(`record["%s"]`, field.sourceField)
		}
		if field.StaticValue != nil {
			src = filter.LuaQuote(*field.StaticValue)
		}

		ra, err := outM.LuaAccessor(true)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %v to Lua accessor: %w", outM, err)
		}
		fmt.Fprintf(&lua, `%s(%s)
`, ra, src)
	}

	// Execute Lua code
	lua.WriteString("return 2, timestamp, record\n")
	lua.WriteString("end\n")
	components = append(components, fluentbit.LuaFilterComponents(tag, "process", lua.String())...)

	// Step 4: Cleanup
	// XXX Remove temporary fields
	// XXX Consider doing this in Lua?
	return components, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorModifyFields{} })
}
