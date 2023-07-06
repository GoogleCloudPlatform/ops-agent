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

package resourcedetector

import "os"

const (
	bmsProjectIDEnv  = "BMS_PROJECT_ID"
	bmsLocationEnv   = "BMS_LOCATION"
	bmsInstanceIDEnv = "BMS_INSTANCE_ID"
)

func testOnBMS() bool {
	return os.Getenv(bmsProjectIDEnv) != "" && os.Getenv(bmsLocationEnv) != "" && os.Getenv(bmsInstanceIDEnv) != ""
}

type BMSMetadataProvider struct{}

func NewBMSMetadataProvider() bmsDataProvider {
	return &BMSMetadataProvider{}
}

func (gmp *BMSMetadataProvider) getProject() string {
	return os.Getenv(bmsProjectIDEnv)
}

func (gmp *BMSMetadataProvider) getLocation() string {
	return os.Getenv(bmsLocationEnv)
}

func (gmp *BMSMetadataProvider) getInstanceID() string {
	return os.Getenv(bmsInstanceIDEnv)
}
