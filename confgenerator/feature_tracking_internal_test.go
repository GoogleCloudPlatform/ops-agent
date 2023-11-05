// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package confgenerator

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type Test struct {
	Name          string
	Config        any
	Expected      []Feature
	ExpectedError error
}

type Primitives struct {
	String                string  `yaml:"string" tracking:""`
	StringWithoutTracking string  `yaml:"stringWithoutTracking"`
	StringWithOverride    string  `yaml:"stringWithOverride" tracking:"override"`
	Bool                  bool    `yaml:"bool"`
	Ptr                   *bool   `yaml:"ptr"`
	PtrWithOverride       *bool   `yaml:"ptrWithOverride" tracking:"override"`
	Int                   int     `yaml:"int"`
	IntWithExclusion      int     `yaml:"int" tracking:"-"`
	Struct                Nested  `yaml:"struct" tracking:"override"`
	Inline                Nested  `yaml:",inline"`
	Invalid               Nested  `yaml:",inline" tracking:"override"`
	PtrToStruct           *Nested `yaml:"ptrStruct" tracking:"override"`
}

type Maps struct {
	Int                   map[string]int               `yaml:"int"`
	IntWithExclusion      map[string]int               `yaml:"intWithExclusion" tracking:"-"`
	String                map[string]string            `yaml:"string" tracking:""`
	StringWithoutTracking map[string]string            `yaml:"stringWithoutTracking"`
	StringWithOverride    map[string]string            `yaml:"stringWithOverride" tracking:"override"`
	Ptr                   map[string]*string           `yaml:"ptr" tracking:""`
	Invalid               map[string]Nested            `yaml:"invalid" tracking:""`
	Struct                map[string]Nested            `yaml:"struct" tracking:"override"`
	StructWithoutTracking map[string]Nested            `yaml:"structWithoutTracking"`
	StructPtr             map[string]*Nested           `yaml:"structPtr"`
	Map                   map[string]map[string]string `yaml:"map" tracking:""`
	Slice                 map[string][]string          `yaml:"slice" tracking:""` //
	MapKeys               map[string]string            `yaml:"keys" tracking:",keys"`
	PtrToMap              *map[string]string           `yaml:"ptrToMap" tracking:""`
}

type Slice struct {
	Int                   []int               `yaml:"int"`
	IntWithExclusion      []int               `yaml:"int" tracking:"-"`
	String                []string            `yaml:"string" tracking:""`
	StringWithoutTracking []string            `yaml:"stringWithoutTracking"`
	StringWithOverride    []string            `yaml:"stringWithOverride" tracking:"override"`
	Ptr                   []*string           `yaml:"ptr" tracking:""`
	Invalid               []Nested            `yaml:"invalid" tracking:""`
	Struct                []Nested            `yaml:"struct" tracking:"override"`
	StructWithoutTracking []Nested            `yaml:"structWithoutTracking"`
	StructPtr             []*Nested           `yaml:"structPtr" tracking:"override"`
	Map                   []map[string]string `yaml:"map" tracking:""`
	Slice                 [][]string          `yaml:"slice" tracking:""`
	MapKeys               []map[string]string `yaml:"keys" tracking:",keys"`
	PtrToSlice            *[]string           `yaml:"ptrToSlice" tracking:""`
}

type Nested struct {
	Int                   int               `yaml:"int"`
	String                string            `yaml:"string" tracking:""`
	StringWithoutTracking string            `yaml:"stringWithoutTracking"`
	Ptr                   *bool             `yaml:"ptr" tracking:"override"`
	Map                   map[string]Nested `yaml:"map"`
	Slice                 []Nested          `yaml:"slice"`
	Nested                *Nested           `yaml:"nested"`
}

func TestBed(t *testing.T) {
	b := true
	foo := "foo"

	tests := []Test{
		{
			Name: "String",
			Config: Primitives{
				String: "foo",
			},
			Expected: []Feature{
				{
					Key:   []string{"string"},
					Value: "foo",
				},
			},
		},
		{
			Name: "StringWithoutTracking",
			Config: Primitives{
				StringWithoutTracking: "foo",
			},
			Expected: nil,
		},
		{
			Name: "StringWithOverride",
			Config: Primitives{
				StringWithOverride: "foo",
			},
			Expected: []Feature{
				{
					Key:   []string{"stringWithOverride"},
					Value: "override",
				},
			},
		},
		{
			Name: "BoolWithAutoTracking",
			Config: Primitives{
				Bool: true,
			},
			Expected: []Feature{
				{
					Key:   []string{"bool"},
					Value: "true",
				},
			},
		},
		{
			Name: "PointerBoolWithAutoTracking",
			Config: Primitives{
				Ptr: &b,
			},
			Expected: []Feature{
				{
					Key:   []string{"ptr"},
					Value: "true",
				},
			},
		},
		{
			Name: "PointerBoolWithOverride",
			Config: Primitives{
				PtrWithOverride: &b,
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrWithOverride"},
					Value: "override",
				},
			},
		},
		{
			Name: "IntWithAutoTracking",
			Config: Primitives{
				Int: 32,
			},
			Expected: []Feature{
				{
					Key:   []string{"int"},
					Value: "32",
				},
			},
		},
		{
			Name: "IntWithExclusion",
			Config: Primitives{
				IntWithExclusion: 32,
			},
			Expected: nil,
		},
		{
			Name: "EmptyStruct",
			Config: Primitives{
				Struct: Nested{},
			},
			Expected: nil,
		},
		{
			Name: "StructIntWithAutoTracking",
			Config: Primitives{
				Struct: Nested{
					Int: 32,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "StructString",
			Config: Primitives{
				Struct: Nested{
					String: "foo",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "string"},
					Value: "foo",
				},
			},
		},
		{
			Name: "StructPtrWithAutoTrackingWithOverride",
			Config: Primitives{
				Struct: Nested{
					Ptr: &b,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "ptr"},
					Value: "override",
				},
			},
		},
		{
			Name: "StructMap",
			Config: Primitives{
				Struct: Nested{
					Map: map[string]Nested{
						"a": {
							Int: 32,
						},
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "map", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"struct", "map", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "StructSlice",
			Config: Primitives{
				Struct: Nested{
					Slice: []Nested{
						{
							Int: 32,
						},
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "slice", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"struct", "slice", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "StructNested",
			Config: Primitives{
				Struct: Nested{
					Nested: &Nested{
						Int: 32,
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "nested", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "InlineStructWithTracking",
			Config: Primitives{
				Inline: Nested{
					Ptr: &b,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptr"},
					Value: "override",
				},
			},
		},
		{
			Name: "Invalid",
			Config: Primitives{
				Invalid: Nested{
					Ptr: &b,
				},
			},
			ExpectedError: ErrTrackingInlineStruct,
		},
		{
			Name: "PointerToStructNil",
			Config: Primitives{
				PtrToStruct: nil,
			},
			Expected: nil,
		},
		{
			Name: "PointerToStructEmpty",
			Config: Primitives{
				PtrToStruct: &Nested{},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrStruct"},
					Value: "override",
				},
			},
		},
		{
			Name: "PointerToStruct",
			Config: Primitives{
				PtrToStruct: &Nested{
					Ptr: &b,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrStruct"},
					Value: "override",
				},
				{
					Key:   []string{"ptrStruct", "ptr"},
					Value: "override",
				},
			},
		},
		{
			Name: "MapIntWithAutoTracking",
			Config: Maps{
				Int: map[string]int{
					"a": 32,
					"b": 64,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"int", "__length"},
					Value: "2",
				},
				{
					Key:   []string{"int", "[0]"},
					Value: "32",
				},
				{
					Key:   []string{"int", "[1]"},
					Value: "64",
				},
			},
		},
		{
			Name: "MapIntWithExclusion",
			Config: Maps{
				IntWithExclusion: map[string]int{
					"a": 32,
					"b": 64,
				},
			},
			Expected: nil,
		},
		{
			Name: "MapString",
			Config: Maps{
				String: map[string]string{
					"foo": "goo",
					"bar": "baz",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"string", "__length"},
					Value: "2",
				},
				{
					Key:   []string{"string", "[0]"},
					Value: "baz",
				},
				{
					Key:   []string{"string", "[1]"},
					Value: "goo",
				},
			},
		},
		{
			Name: "MapStringWithoutTracking",
			Config: Maps{
				StringWithoutTracking: map[string]string{
					"foo": "goo",
					"bar": "baz",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"stringWithoutTracking", "__length"},
					Value: "2",
				},
			},
		},
		{
			Name: "MapStringWithOverride",
			Config: Maps{
				StringWithOverride: map[string]string{
					"foo": "goo",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"stringWithOverride", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"stringWithOverride", "[0]"},
					Value: "override",
				},
			},
		},
		{
			Name: "MapPtr",
			Config: Maps{
				Ptr: map[string]*string{
					"foo": &foo,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptr", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"ptr", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "MapInvalid",
			Config: Maps{
				Invalid: map[string]Nested{
					"a": {
						Int: 32,
					},
				},
			},
			ExpectedError: ErrTrackingOverrideStruct,
		},
		{
			Name: "MapStruct",
			Config: Maps{
				Struct: map[string]Nested{
					"a": {
						Int: 32,
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"struct", "[0]"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "StructWithoutTrackingAutoTracking",
			Config: Maps{
				StructWithoutTracking: map[string]Nested{
					"a": {
						Int:                   32,
						String:                "foo",
						StringWithoutTracking: "goo",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structWithoutTracking", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"structWithoutTracking", "[0]", "int"},
					Value: "32",
				},
				{
					Key:   []string{"structWithoutTracking", "[0]", "string"},
					Value: "foo",
				},
			},
		},
		{
			Name: "StructPtrNil",
			Config: Maps{
				StructPtr: map[string]*Nested{
					"a": nil,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structPtr", "__length"},
					Value: "1",
				},
			},
		},
		{
			Name: "StructPtr",
			Config: Maps{
				StructPtr: map[string]*Nested{
					"a": {
						Int: 32,
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structPtr", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"structPtr", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "MapMap",
			Config: Maps{
				Map: map[string]map[string]string{
					"a": {
						"b": "foo",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"map", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"map", "[0]", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"map", "[0]", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "MapSliceNil",
			Config: Maps{
				Slice: map[string][]string{
					"a": nil,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"slice", "__length"},
					Value: "1",
				},
			},
		},
		{
			Name: "MapSliceEmpty",
			Config: Maps{
				Slice: map[string][]string{
					"a": {},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"slice", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"slice", "[0]", "__length"},
					Value: "0",
				},
			},
		},
		{
			Name: "MapSlice",
			Config: Maps{
				Slice: map[string][]string{
					"a": {"foo"},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"slice", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"slice", "[0]", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"slice", "[0]", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "MapKeys",
			Config: Maps{
				MapKeys: map[string]string{
					"a": "foo",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"keys", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"keys", "[0]", "__key"},
					Value: "a",
				},
				{
					Key:   []string{"keys", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "PtrToMapNil",
			Config: Maps{
				PtrToMap: nil,
			},
			Expected: nil,
		},
		{
			Name: "PtrToMap",
			Config: Maps{
				PtrToMap: &map[string]string{
					"a": "foo",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrToMap", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"ptrToMap", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "SliceIntWithAutoTracking",
			Config: Slice{
				Int: []int{1, 2, 3},
			},
			Expected: []Feature{
				{
					Key:   []string{"int", "__length"},
					Value: "3",
				},
				{
					Key:   []string{"int", "[0]"},
					Value: "1",
				},
				{
					Key:   []string{"int", "[1]"},
					Value: "2",
				},
				{
					Key:   []string{"int", "[2]"},
					Value: "3",
				},
			},
		},
		{
			Name: "SliceIntWithExclusion",
			Config: Slice{
				IntWithExclusion: []int{1, 2, 3},
			},
			Expected: nil,
		},
		{
			Name: "SliceString",
			Config: Slice{
				String: []string{"foo", "goo", "bar"},
			},
			Expected: []Feature{
				{
					Key:   []string{"string", "__length"},
					Value: "3",
				},
				{
					Key:   []string{"string", "[0]"},
					Value: "foo",
				},
				{
					Key:   []string{"string", "[1]"},
					Value: "goo",
				},
				{
					Key:   []string{"string", "[2]"},
					Value: "bar",
				},
			},
		},
		{
			Name: "SliceStringWithoutTracking",
			Config: Slice{
				StringWithoutTracking: []string{"foo", "goo", "bar"},
			},
			Expected: []Feature{
				{
					Key:   []string{"stringWithoutTracking", "__length"},
					Value: "3",
				},
			},
		},
		{
			Name: "SliceStringWithOverride",
			Config: Slice{
				StringWithOverride: []string{"foo", "goo", "bar"},
			},
			Expected: []Feature{
				{
					Key:   []string{"stringWithOverride", "__length"},
					Value: "3",
				},
				{
					Key:   []string{"stringWithOverride", "[0]"},
					Value: "override",
				},
				{
					Key:   []string{"stringWithOverride", "[1]"},
					Value: "override",
				},
				{
					Key:   []string{"stringWithOverride", "[2]"},
					Value: "override",
				},
			},
		},
		{
			Name: "SlicePtr",
			Config: Slice{
				Ptr: []*string{&foo},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptr", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"ptr", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "SliceInvalid",
			Config: Slice{
				Invalid: []Nested{
					{
						Int: 32,
					},
				},
			},
			ExpectedError: ErrTrackingOverrideStruct,
		},
		{
			Name: "SliceStruct",
			Config: Slice{
				Struct: []Nested{
					{
						Int: 32,
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"struct", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"struct", "[0]"},
					Value: "override",
				},
				{
					Key:   []string{"struct", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "SliceStructWithoutTracking",
			Config: Slice{
				StructWithoutTracking: []Nested{
					{
						Int:                   32,
						String:                "foo",
						StringWithoutTracking: "goo",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structWithoutTracking", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"structWithoutTracking", "[0]", "int"},
					Value: "32",
				},
				{
					Key:   []string{"structWithoutTracking", "[0]", "string"},
					Value: "foo",
				},
			},
		},
		{
			Name: "SliceStructPtrNil",
			Config: Slice{
				StructPtr: nil,
			},
			Expected: nil,
		},
		{
			Name: "SliceStructPtrEmpty",
			Config: Slice{
				StructPtr: []*Nested{
					nil,
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structPtr", "__length"},
					Value: "1",
				},
			},
		},
		{
			Name: "SliceStructPtr",
			Config: Slice{
				StructPtr: []*Nested{
					{
						Int: 32,
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"structPtr", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"structPtr", "[0]"},
					Value: "override",
				},
				{
					Key:   []string{"structPtr", "[0]", "int"},
					Value: "32",
				},
			},
		},
		{
			Name: "SliceMap",
			Config: Slice{
				Map: []map[string]string{
					{
						"a": "foo",
					},
					{
						"b": "bar",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"map", "__length"},
					Value: "2",
				},
				{
					Key:   []string{"map", "[0]", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"map", "[0]", "[0]"},
					Value: "foo",
				},
				{
					Key:   []string{"map", "[1]", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"map", "[1]", "[0]"},
					Value: "bar",
				},
			},
		},
		{
			Name: "SliceSlice",
			Config: Slice{
				Slice: [][]string{
					{
						"foo",
						"goo",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"slice", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"slice", "[0]", "__length"},
					Value: "2",
				},
				{
					Key:   []string{"slice", "[0]", "[0]"},
					Value: "foo",
				},
				{
					Key:   []string{"slice", "[0]", "[1]"},
					Value: "goo",
				},
			},
		},
		{
			Name: "SliceMapKeys",
			Config: Slice{
				MapKeys: []map[string]string{
					{
						"a": "foo",
					},
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"keys", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"keys", "[0]", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"keys", "[0]", "[0]", "__key"},
					Value: "a",
				},
				{
					Key:   []string{"keys", "[0]", "[0]"},
					Value: "foo",
				},
			},
		},
		{
			Name: "SlicePtrToSliceNil",
			Config: Slice{
				PtrToSlice: nil,
			},
			Expected: nil,
		},
		{
			Name: "SlicePtrToSliceEmpty",
			Config: Slice{
				PtrToSlice: &[]string{},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrToSlice", "__length"},
					Value: "0",
				},
			},
		},
		{
			Name: "SlicePtrToSlice",
			Config: Slice{
				PtrToSlice: &[]string{
					"foo",
				},
			},
			Expected: []Feature{
				{
					Key:   []string{"ptrToSlice", "__length"},
					Value: "1",
				},
				{
					Key:   []string{"ptrToSlice", "[0]"},
					Value: "foo",
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			actual, err := trackingFeatures(reflect.ValueOf(test.Config), metadata{}, Feature{})

			if test.ExpectedError != nil {
				if test.Expected != nil {
					t.Fatalf("invalid test: %v", test.Name)
				}
				if !errors.Is(err, test.ExpectedError) {
					t.Fatal(err)
				}
			} else {
				expected := test.Expected
				if !cmp.Equal(actual, expected) {
					t.Fatalf("expected: %v, actual: %v, \ndiff: %v", expected, actual, cmp.Diff(expected, actual))
				}
			}
		})
	}
}
