// Copyright 2023 Google LLC
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

package secret

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

var RedactedValue string = "__redacted__"

type Secret[T any] interface {
	fmt.Stringer
	yaml.BytesMarshaler
	SecretValue() T
}

type String string

// fmt.Stringer
func (s String) String() string {
	return RedactedValue
}

// yaml.BytesMarshaler
func (s String) MarshalYAML() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s String) SecretValue() string {
	return string(s)
}
