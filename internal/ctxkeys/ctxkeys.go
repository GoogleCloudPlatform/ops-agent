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

package ctxkeys

import "context"

type key int

const (
	otlpExporter key = iota
)

func ContextWithOtlpExporter(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, otlpExporter, enabled)
}

func OtlpExporterFromContext(ctx context.Context) bool {
	if enabled, ok := ctx.Value(otlpExporter).(bool); ok {
		return enabled
	}
	return false
}
