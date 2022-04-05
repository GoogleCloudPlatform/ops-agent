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

// Package modify provides helpers for generating fluentbit configs
package modify

import (
	"fmt"

	"github.com/GoogleCloudPlatform/ops-agent/confgenerator/fluentbit"
)

// ModifyRule is a string corresponding to one of the Configuration Rules of fluentbit
// for a full list please refer to https://docs.fluentbit.io/manual/pipeline/filters/modify#configuration-parameters
type ModifyRule string

const (
	// Add a key/value pair with key KEY and value VALUE if KEY does not exist
	AddModifyKey            ModifyRule = "Add"
	SetModifyKey            ModifyRule = "Set"
	RemoveModifyKey         ModifyRule = "Remove"
	RemoveWildcardModifyKey ModifyRule = "Remove_wildcard"
	RemoveRegexModifyKey    ModifyRule = "Remove_regex"
	RenameModifyKey         ModifyRule = "Rename"
	HardRenameModifyKey     ModifyRule = "Hard_rename"
	CopyModifyKey           ModifyRule = "Copy"
	HardCopyModifyKey       ModifyRule = "Hard_copy"
)

// ModifyConditionKey is a string corresponding to a name of the modify filter's
// conditional expressions. For a full list please refer to https://docs.fluentbit.io/manual/pipeline/filters/modify#conditions
type ModifyConditionKey string

const (
	ModifyConditionKeyExists       ModifyConditionKey = "Key_exists"
	ModifyConditionKeyDoesNotExist ModifyConditionKey = "Key_does_not_exist"
	ModifyConditionKeyValueEquals  ModifyConditionKey = "Key_value_equals"
	ModifyConditionNoKeyMatches    ModifyConditionKey = "No_key_matches"
	ModifyKeyValueMatches          ModifyConditionKey = "Key_value_matches"
)

// ModifyOptions are a representation of a config for a modify block in fluentbit
type ModifyOptions struct {
	ModifyRule ModifyRule
	// Parameters is the string input of the modify rule
	// i.e "Rename timestamp time"; Parameters = "timestamp time"
	Parameters string
}

// Component uses the option and transforms it into a Component
func (mo ModifyOptions) Component(tag string) fluentbit.Component {
	c := fluentbit.Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":  "modify",
			"Match": tag,
		},
	}
	c.Config[string(mo.ModifyRule)] = mo.Parameters

	return c
}

// MapModify takes a list of ModifyOptions and converts them to Modify components
// and returns a slice of them
func MapModify(tag string, modifications []ModifyOptions) []fluentbit.Component {
	c := []fluentbit.Component{}
	for _, m := range modifications {
		c = append(c, m.Component(tag))
	}
	return c
}

// NewSetModifyOptions creates the ModifyOptions that will construct a Set modify
// where the `field` is set to the `value` parameter. Note this will overwrite if field
// already exists
func NewSetOptions(field, value string) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: SetModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, value),
	}
	return mo
}

// NewRenameModifyOptions creates the ModifyOptions that on `Component()` will construct a Rename
// fluentbit component. Note that Rename does not overwrite fields if they exist
func NewRenameOptions(field, renameTo string) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: RenameModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, renameTo),
	}
	return mo
}

// NewHardRenameModifyOptions creates the ModifyOptions that on `Component()` will return
// a fluentbit component that does a hard rename of `field` to `renameTo`. Note that this will overwrite
// the current value of field if it does exist.
func NewHardRenameOptions(field, renameTo string) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: HardRenameModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, renameTo),
	}
	return mo
}
