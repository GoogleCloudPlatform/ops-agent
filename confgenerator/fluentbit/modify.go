package fluentbit

import "fmt"

// ModifyRule is a string corresponding to one of the Configuration Rules of fluentbit
// for a full list please refer to https://docs.fluentbit.io/manual/pipeline/filters/modify#configuration-parameters
type ModifyRule = string

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
type ModifyConditionKey = string

const (
	ModifyConditionKeyExists       ModifyConditionKey = "Key_exists"
	ModifyConditionKeyDoesNotExist ModifyConditionKey = "Key_does_not_exist"
	ModifyConditionKeyValueEquals  ModifyConditionKey = "Key_value_equals"
	ModifyConditionNoKeyMatches    ModifyConditionKey = "No_key_matches"
	ModifyKeyValueMatches          ModifyConditionKey = "Key_value_matches"
)

// ModifyCondition is a representation of a fluentbit modify condition which
// evaluates an expression in order to determine if the modify should be done
type ModifyCondition struct {
	Name ModifyConditionKey
	Key  string
	// if condition does not require a parameter, omit or use zero value: ""
	Value string
}

// Expression returns the expression string for a conditional modify statement
func (mc *ModifyCondition) Expression() string {
	expr := fmt.Sprintf("%s %s", mc.Name, mc.Key)
	if mc.Value != "" {
		expr += fmt.Sprintf(" %s", mc.Value)
	}
	return expr
}

// ModifyOptions are a representation of a config for a modify block in fluentbit
type ModifyOptions struct {
	ModifyRule ModifyRule
	Condition  *ModifyCondition
	// Parameters is the string input of the modify rule
	// i.e "Rename timestamp time"; Parameters = "timestamp time"
	Parameters string
}

// Component uses the option and transforms it into a Component
func (mo *ModifyOptions) Component(tag string) Component {
	c := Component{
		Kind: "FILTER",
		Config: map[string]string{
			"Name":  "modify",
			"Match": tag,
		},
	}
	c.Config[mo.ModifyRule] = mo.Parameters

	if mo.Condition != nil {
		c.Config["Condition"] = mo.Condition.Expression()
	}
	return c
}

// MapModify takes a list of ModifyOptions and converts them to Modify components
// and returns a slice of them
func MapModify(tag string, modifications []ModifyOptions) []Component {
	c := []Component{}
	for _, m := range modifications {
		c = append(c, m.Component(tag))
	}
	return c
}

// NewSetModifyOptions creates the ModifyOptions that will construct a Set modify
// where the `field` is set to the `value` parameter. Note this will overwrite if field
// already exists
func NewSetModifyOptions(field, value string, condition *ModifyCondition) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: SetModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, value),
	}
	if condition != nil {
		mo.Condition = condition
	}
	return mo
}

// NewRenameModifyOptions creates the ModifyOptions that on `Component()` will construct a Rename
// fluentbit component. Note that Rename does not overwrite fields if they exist
func NewRenameModifyOptions(field, renameTo string, condition *ModifyCondition) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: RenameModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, renameTo),
	}
	if condition != nil {
		mo.Condition = condition
	}
	return mo
}

// NewHardRenameModifyOptions creates the ModifyOptions that on `Component()` will return
// a fluentbit component that does a hard rename of `field` to `renameTo`. Note that this will overwrite
// the current value of field if it does exist.
func NewHardRenameModifyOptions(field, renameTo string, condition *ModifyCondition) ModifyOptions {
	mo := ModifyOptions{
		ModifyRule: HardRenameModifyKey,
		Parameters: fmt.Sprintf("%s %s", field, renameTo),
	}
	if condition != nil {
		mo.Condition = condition
	}
	return mo
}
