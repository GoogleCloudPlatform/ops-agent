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
	MoveFrom     string  `yaml:"move_from" validate:"omitempty,field,excluded_with=CopyFrom StaticValue"`
	CopyFrom     string  `yaml:"copy_from" validate:"omitempty,field,excluded_with=MoveFrom StaticValue"`
	StaticValue  *string `yaml:"static_value" validate:"excluded_with=MoveFrom CopyFrom DefaultValue"`
	DefaultValue *string `yaml:"default_value" validate:"excluded_with=StaticValue"`

	// Name of Lua variable with copied value
	sourceVar string `yaml:"-"`
	// Name of Lua variable with omit boolean
	omitVar string `yaml:"-"`

	// Operations to perform
	MapValues map[string]string `yaml:"map_values"`
	Type      string            `yaml:"type" validate:"omitempty,oneof=integer float"`
	OmitIf    string            `yaml:"omit_if" validate:"omitempty,filter"`
}

type LoggingProcessorModifyFields struct {
	ConfigComponent `yaml:",inline"`
	Fields          map[string]*ModifyField `yaml:"fields" validate:"dive,keys,field,distinctfield,endkeys"`
}

func (p LoggingProcessorModifyFields) Type() string {
	return "modify_fields"
}

func (p LoggingProcessorModifyFields) Components(tag, uid, platform string) []fluentbit.Component {
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
	fieldMappings := map[string]string{}
	moveFromFields := map[string]bool{}
	var dests []string
	for dest, field := range p.Fields {
		if field == nil {
			// Nothing to do for this field
			continue
		}
		dests = append(dests, dest)
	}
	sort.Strings(dests)
	omitFilters := map[string]*filter.Filter{}
	for i, dest := range dests {
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
				fmt.Fprintf(&lua, "local %s = %s;\n", new, key)
			}
			field.sourceVar = fieldMappings[key]
			if j == 0 {
				ra, err := m.LuaAccessor(true)
				if err != nil {
					return nil, fmt.Errorf("failed to convert %v to Lua accessor: %w", m, err)
				}
				moveFromFields[ra] = true
			}
		}
		if field.OmitIf != "" {
			f, err := filter.NewFilter(field.OmitIf)
			if err != nil {
				return nil, fmt.Errorf("failed to parse filter %q: %w", field.OmitIf, err)
			}
			field.omitVar = fmt.Sprintf("omit%d", i)
			omitFilters[field.omitVar] = f
		}
	}

	// Step 2: OmitIf conditions
	if len(omitFilters) > 0 {
		fcomponents, flua := filter.AllFluentConfig(tag, omitFilters)
		components = append(components, fcomponents...)
		lua.WriteString(flua)
	}

	// Step 3: Remove any MoveFrom fields
	for ra := range moveFromFields {
		fmt.Fprintf(&lua, `%s(nil);
`, ra)
	}
	// Step 4: Assign values
	for _, dest := range dests {
		field := p.Fields[dest]
		outM, err := filter.NewMember(dest)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output field %q: %m", dest, err)
		}

		src := "nil"
		if field.sourceVar != "" {
			src = field.sourceVar
		}
		if field.StaticValue != nil {
			src = filter.LuaQuote(*field.StaticValue)
		}

		fmt.Fprintf(&lua, "local v = %s;\n", src)

		if field.DefaultValue != nil {
			fmt.Fprintf(&lua, "if v == nil then v = %s end;\n", filter.LuaQuote(*field.DefaultValue))
		}

		// Process MapValues

		var keys []string
		for k := range field.MapValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				lua.WriteString("else")
			}
			fmt.Fprintf(&lua, "if v == %s then v = %s\n", filter.LuaQuote(k), filter.LuaQuote(field.MapValues[k]))
		}
		if len(keys) > 0 {
			lua.WriteString("end\n")
		}

		// Process Type
		var conv string
		switch field.Type {
		case "integer":
			// Fluent-bit currently targets Lua 5.1, which uses the same type for numbers and integers.
			// When converting back to msgpack, if a number can be represented as an integer, fluent-bit does so, otherwise it uses a float.
			// If fluent-bit ever supports Lua 5.3, we can switch this to math.tointeger and use proper integers.
			conv = "math.floor(tonumber(v))"
		case "float":
			conv = "tonumber(v)"
		}
		if conv != "" {
			// Leave existing string value if not convertible
			fmt.Fprintf(&lua, `
local v2 = %s
if v2 ~= fail then v = v2
end
`, conv)
		}

		// Omit if
		if field.omitVar != "" {
			fmt.Fprintf(&lua, "if %s then v = nil end;\n", field.omitVar)
		}

		ra, err := outM.LuaAccessor(true)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %v to Lua accessor: %w", outM, err)
		}
		fmt.Fprintf(&lua, "%s(v)\n", ra)
	}

	lua.WriteString("return 2, timestamp, record\n")
	lua.WriteString("end\n")

	// Execute Lua code
	components = append(components, fluentbit.LuaFilterComponents(tag, "process", lua.String())...)

	return components, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() Component { return &LoggingProcessorModifyFields{} })
}
