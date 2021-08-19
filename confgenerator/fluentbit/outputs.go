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

package fluentbit

import "fmt"

func (o Stackdriver) Component() Component {
	config := map[string]string{
		// https://docs.fluentbit.io/manual/pipeline/outputs/stackdriver
		"Name":              "stackdriver",
		"Match_Regex":       fmt.Sprintf("^(%s)$", o.Match), // FIXME: properly escape, join from list
		"resource":          "gce_instance",
		"stackdriver_agent": o.UserAgent,

		// https://docs.fluentbit.io/manual/administration/scheduling-and-retries
		// After 3 retries, a given chunk will be discarded. So bad entries don't accidentally stay around forever.
		"Retry_Limit": "3",

		// https://docs.fluentbit.io/manual/administration/security
		// Enable TLS support.
		"tls": "On",
		// Do not force certificate validation.
		"tls.verify": "Off",
	}
	if o.Workers != 0 {
		config["workers"] = fmt.Sprintf("%d", o.Workers)
	}
	return Component{
		Kind:   "OUTPUT",
		Config: config,
	}
}
