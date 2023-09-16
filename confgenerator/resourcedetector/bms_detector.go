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

package resourcedetector

import (
	"fmt"
	"os"
)

const (
	bmsProjectIDEnv  = "BMS_PROJECT_ID"
	bmsLocationEnv   = "BMS_LOCATION"
	bmsInstanceIDEnv = "BMS_INSTANCE_ID"
)

// Keep this in-sync with https://github.com/GoogleCloudPlatform/opentelemetry-operations-go/blob/54a8992128b1936db270ecf1e8360c62ba17936c/internal/resourcemapping/resourcemapping.go#L182C33-L182C33
const BMSCloudPlatformAttribute = "gcp_bare_metal_solution"

// BMSResource implements the Resource interface and provides attributes of a BMS instance
type BMSResource struct {
	Project    string
	InstanceID string
	Location   string
}

func (BMSResource) GetType() string {
	return "baremetalsolution.googleapis.com/Instance"
}

func (r BMSResource) GetProject() string {
	return r.Project
}

func (r BMSResource) OTelResourceAttributes() map[string]string {
	return map[string]string{
		"cloud.platform": BMSCloudPlatformAttribute,
		"cloud.project":  r.Project,
		"cloud.region":   r.Location,
		"host.id":        r.InstanceID,
	}
}

func (r BMSResource) FluentBitLabels() string {
	return fmt.Sprintf("resource_container=%s,location=%s,instance_id=%s", r.Project, r.Location, r.InstanceID)
}

func (r BMSResource) GetLabels() map[string]string {
	return map[string]string{
		"location":    r.Location,
		"instance_id": r.InstanceID,
	}
}

func OnBMS() bool {
	return os.Getenv(bmsProjectIDEnv) != "" && os.Getenv(bmsLocationEnv) != "" && os.Getenv(bmsInstanceIDEnv) != ""
}

func GetBMSResource() (Resource, error) {
	return BMSResource{
		Project:    os.Getenv(bmsProjectIDEnv),
		Location:   os.Getenv(bmsLocationEnv),
		InstanceID: os.Getenv(bmsInstanceIDEnv),
	}, nil
}
