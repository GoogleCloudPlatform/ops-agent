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

package set

type Set[T comparable] map[T]struct{}

func ToSet[K comparable, V any](m map[K]V) Set[K] {
	s := map[K]struct{}{}
	for k := range m {
		s[k] = struct{}{}
	}
	return s
}

func (s Set[T]) Add(k T) {
	s[k] = struct{}{}
}

func (s Set[T]) Contains(k T) bool {
	_, ok := s[k]
	return ok
}

func (s Set[T]) Keys() []T {
	result := make([]T, len(s))
	i := 0
	for k := range s {
		result[i] = k
		i++
	}
	return result
}
