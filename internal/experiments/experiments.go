// Copyright 2026 Google LLC
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

package experiments

import (
	"context"
	"os"
	"strings"
)

var enabledExperimentalFeatures map[string]bool

// ParseExperimentalFeatures parses a comma-separated list of features.
func ParseExperimentalFeatures(features string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.Split(features, ",") {
		out[strings.TrimSpace(f)] = true
	}
	return out
}

func init() {
	enabledExperimentalFeatures = ParseExperimentalFeatures(os.Getenv("EXPERIMENTAL_FEATURES"))
}

type experimentsKeyType struct{}

var experimentsKey = experimentsKeyType{}

// ContextWithExperiments returns a new context with the given experiments enabled.
func ContextWithExperiments(ctx context.Context, experiments map[string]bool) context.Context {
	return context.WithValue(ctx, experimentsKey, experiments)
}

// FromContext returns the enabled experiments from the context, or the default enabled ones from the environment.
func FromContext(ctx context.Context) map[string]bool {
	if features := ctx.Value(experimentsKey); features != nil {
		return features.(map[string]bool)
	}
	return enabledExperimentalFeatures
}
