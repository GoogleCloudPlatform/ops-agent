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
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

type ModifyField struct {
	// Source of value for this field
	// XXX: Validate field names
	MoveFrom    string  `yaml:"move_from" validate:"excluded_with=CopyFrom StaticValue"`
	CopyFrom    string  `yaml:"copy_from" validate:"excluded_with=MoveFrom StaticValue"`
	StaticValue *string `yaml:"static_value" validate:"excluded_with=MoveFrom CopyFrom"`

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
	Fields          map[string]*ModifyField `yaml:"fields"`
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
	var dests []string
	for dest := range p.Fields {
		dests = append(dests, dest)
	}
	sort.Strings(dests)
	for _, dest := range dests {
		field := p.Fields[dest]
		if field.MoveFrom == "" && field.CopyFrom == "" && field.StaticValue == nil {
			// Default to modifying field in place
			field.CopyFrom = dest
		}
		for j, name := range []*string{&field.MoveFrom, &field.CopyFrom} {
			if *name == "" {
				continue
			}
			m, err := filter.NewMember(*name)
			if err != nil {
				return nil, fmt.Errorf("failed to parse field %q: %w", *name, err)
			}
			key, err := m.LuaAccessor(false)
			if err != nil {
				return nil, fmt.Errorf("failed to convert field %q to Lua accessor: %w", *name, err)
			}
			if _, ok := fieldMappings[key]; !ok {
				new := fmt.Sprintf("__field_%d", i)
				fieldMappings[key] = new
				i++
				fmt.Fprintf(&lua, "local %s = %s;\n", new, key)
			}
			field.sourceField = fieldMappings[key]
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
	for _, dest := range dests {
		field := p.Fields[dest]
		outM, err := filter.NewMember(dest)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output field %q: %m", dest, err)
		}

		src := "nil"
		if field.sourceField != "" {
			src = fmt.Sprintf(`record["%s"]`, field.sourceField)
		}
		if field.StaticValue != nil {
			src = filter.LuaQuote(*field.StaticValue)
		}

		fmt.Fprintf(&lua, "local v = %s;\n", src)

		// Process MapValues

		i := 0
		// TODO: Iterate in a deterministic order
		for k, v := range field.MapValues {
			if i > 0 {
				lua.WriteString("else")
			}
			fmt.Fprintf(&lua, "if v == %s then v = %s\n", filter.LuaQuote(k), filter.LuaQuote(v))
			i++
		}
		if i > 0 {
			lua.WriteString("end\n")
		}

		// Process Type
		var conv string
		switch field.Type {
		case "integer":
			conv = "math.tointeger"
		case "float":
			conv = "tonumber"
		}
		if conv != "" {
			// Leave existing string value if not convertible
			fmt.Fprintf(&lua, `
local v2 = %s(v)
if v2 != fail then v = v2
end
`, conv)
		}

		ra, err := outM.LuaAccessor(true)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %v to Lua accessor: %w", outM, err)
		}
		fmt.Fprintf(&lua, "%s(v)\n", ra)
	}

	// Step 4: Cleanup
	// Remove temporary fields
	lua.WriteString(`
for k,v in pairs(record) do
  if string.match(k, "^__field.+") or string.match(k, "^__match.+") then
    record[k] = nil
  end
end
return 2, timestamp, record
end
`)
	// Execute Lua code
	components = append(components, fluentbit.LuaFilterComponents(tag, "process", lua.String())...)

	return components, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorModifyFields{} })
}
