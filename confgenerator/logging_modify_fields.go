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
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/filter"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel"
	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/otel/ottl"
)

type ModifyField struct {
	// Source of value for this field
	MoveFrom     string  `yaml:"move_from" validate:"omitempty,field,excluded_with=CopyFrom StaticValue"`
	CopyFrom     string  `yaml:"copy_from" validate:"omitempty,field,excluded_with=MoveFrom StaticValue"`
	StaticValue  *string `yaml:"static_value" validate:"excluded_with=MoveFrom CopyFrom DefaultValue"`
	DefaultValue *string `yaml:"default_value" validate:"excluded_with=StaticValue"`

	// OTTL expression with copied value
	sourceValue ottl.Value `yaml:"-"`
	// Name of OTTL cache variable with omit boolean
	omitVar string `yaml:"-"`

	// Operations to perform
	Type              string                            `yaml:"type" validate:"omitempty,oneof=integer float"`
	CustomConvertFunc func(ottl.LValue) ottl.Statements `yaml:"-"`
	OmitIf            string                            `yaml:"omit_if" validate:"omitempty,filter"`
	MapValues         map[string]string                 `yaml:"map_values"`
	// In case the source field's value does not match any keys specified in the map_values pairs,
	// the destination field will be forcefully unset if map_values_exclusive is true,
	// or left untouched if map_values_exclusive is false.
	MapValuesExclusive bool `yaml:"map_values_exclusive" validate:"excluded_without=MapValues"`
}

type LoggingProcessorModifyFields struct {
	ConfigComponent `yaml:",inline"`
	Fields          map[string]*ModifyField `yaml:"fields" validate:"dive,keys,field,distinctfield,writablefield,endkeys" tracking:"-"`

	// For use by other processors, if set this will clear out `jsonPayload`, leaving only the fields set above.
	// Only supported in OTel.
	EmptyBody bool `yaml:"-" tracking:"-"`
}

func (p LoggingProcessorModifyFields) Type() string {
	return "modify_fields"
}

func (p LoggingProcessorModifyFields) Processors(ctx context.Context) ([]otel.Component, error) {
	out, err := p.statements(ctx)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return []otel.Component{otel.Transform(
		"log", "log",
		out,
	)}, nil
}

func (p LoggingProcessorModifyFields) statements(_ context.Context) (ottl.Statements, error) {
	var statements ottl.Statements

	var dests []string
	for dest, field := range p.Fields {
		if field == nil {
			// Nothing to do for this field
			continue
		}
		dests = append(dests, dest)
	}
	sort.Strings(dests)

	// map of (dest field as OTTL expression) to (source field as OTTL expression)
	fieldMappings := map[string]ottl.LValue{}
	// slice of OTTL fields to delete
	var moveFromFields []ottl.LValue
	// map of (variable name) to (filter object)
	var omitFilters []*filter.Filter

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
			accessor, err := m.OTTLAccessor()
			if err != nil {
				return nil, fmt.Errorf("failed to convert field %q to OTTL: %w", *name, err)
			}
			key := accessor.String()
			if _, ok := fieldMappings[key]; !ok {
				new := ottl.LValue{
					"cache",
					fmt.Sprintf("__field_%d", i),
				}
				fieldMappings[key] = new
				statements = statements.Append(
					new.Delete(),
					new.SetIf(accessor, accessor.IsPresent()),
				)
			}
			field.sourceValue = fieldMappings[key]
			if j == 0 { // MoveFrom
				moveFromFields = append(moveFromFields, accessor)
			}
		}
		if field.OmitIf != "" {
			f, err := filter.NewFilter(field.OmitIf)
			if err != nil {
				return nil, fmt.Errorf("failed to parse filter %q: %w", field.OmitIf, err)
			}
			field.omitVar = fmt.Sprintf("__omit_%d", len(omitFilters))
			omitFilters = append(omitFilters, f)
		}
	}
	// Step 2: OmitIf conditions
	for i, f := range omitFilters {
		name := fmt.Sprintf("__omit_%d", i)
		expr, err := f.OTTLExpression()
		if err != nil {
			return nil, fmt.Errorf("failed to parse omit_if condition %q: %w", f, err)
		}
		statements = statements.Append(
			ottl.LValue{"cache", name}.Set(ottl.False()),
			ottl.LValue{"cache", name}.SetIf(ottl.True(), expr),
		)
	}

	// Step 3: Remove any MoveFrom fields
	// Sort first to make the resulting configs deterministic
	sort.Slice(moveFromFields, func(i, j int) bool {
		return moveFromFields[i].String() < moveFromFields[j].String()
	})
	var last ottl.LValue
	for _, v := range moveFromFields {
		if !slices.Equal(last, v) {
			statements = statements.Append(v.Delete())
		}
		last = v
	}

	if p.EmptyBody {
		// With no keys, will clear out the body, leaving a copy at cache.body.
		statements = statements.Append(
			ottl.LValue{"cache", "body"}.Set(ottl.LValue{"body"}),
			ottl.LValue{"body"}.KeepKeys(),
		)
	}

	// Step 4: Assign values
	for _, dest := range dests {
		field := p.Fields[dest]
		outM, err := filter.NewMember(dest)
		if err != nil {
			return nil, fmt.Errorf("failed to parse output field %q: %w", dest, err)
		}

		src := ottl.Nil()
		if field.sourceValue != nil {
			src = field.sourceValue
		}
		if field.StaticValue != nil {
			src = ottl.StringLiteral(*field.StaticValue)
		}
		value := ottl.LValue{"cache", "value"}
		statements = statements.Append(
			// Set silently fails to set if the value is nil, so we delete first.
			value.Delete(),
			value.Set(src),
		)
		if field.DefaultValue != nil {
			statements = statements.Append(
				value.SetIfNil(ottl.StringLiteral(*field.DefaultValue)),
			)
		}

		// Process MapValues
		if len(field.MapValues) > 0 {
			mapped_value := ottl.LValue{"cache", "mapped_value"}
			statements = statements.Append(
				mapped_value.Delete(),
			)
			if !field.MapValuesExclusive {
				statements = statements.Append(
					mapped_value.SetIf(value, value.IsPresent()),
				)
			}
			var keys []string
			for k := range field.MapValues {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				statements = statements.Append(
					mapped_value.SetIf(ottl.StringLiteral(field.MapValues[k]), ottl.Equals(value, ottl.StringLiteral(k))),
				)
			}

			value = mapped_value
		}
		switch field.Type {
		case "integer":
			statements = statements.Append(value.Set(ottl.ToInt(value)))
		case "float":
			statements = statements.Append(value.Set(ottl.ToFloat(value)))
		case "YesNoBoolean":
			statements = statements.Append(value.SetToYesNoBoolean(value))
		}

		if field.CustomConvertFunc != nil {
			statements = statements.Append(field.CustomConvertFunc(value))
		}

		ra, err := outM.OTTLAccessor()
		if err != nil {
			return nil, fmt.Errorf("failed to convert %v to OTTL accessor: %w", outM, err)
		}
		statements = statements.Append(ra.SetIf(value, value.IsPresent()))

		if field.omitVar != "" {
			statements = statements.Append(ra.DeleteIf(ottl.Equals(ottl.LValue{"cache", field.omitVar}, ottl.True())))
		}
	}
	return statements, nil
}

func (p LoggingProcessorModifyFields) ListAllFeatures() ([]string, bool) {
	return []string{
		"has_ruby_regex",
	}, false
}

func (p LoggingProcessorModifyFields) ExtractFeatures() ([]CustomFeature, bool, error) {
	hasRubyRegex := false
	for _, field := range p.Fields {
		if field != nil && field.OmitIf != "" {
			flt, err := filter.NewFilter(field.OmitIf)
			if err != nil {
				// Unparsable; just skip the feature.
				return nil, false, nil
			}
			if flt.HasRubyRegex() {
				hasRubyRegex = true
				break
			}
		}
	}
	return []CustomFeature{
		{
			Key:   []string{"has_ruby_regex"},
			Value: fmt.Sprintf("%v", hasRubyRegex),
		},
	}, false, nil
}

func init() {
	LoggingProcessorTypes.RegisterType(func() LoggingProcessor { return &LoggingProcessorModifyFields{} })
}
