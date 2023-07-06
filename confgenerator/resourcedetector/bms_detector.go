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

func GetBMSResource() (Resource, error) {
	provider := NewBMSMetadataProvider()
	dt := BMSResourceBuilder{provider: provider}
	return dt.GetResource()
}

// BMSResource implements the Resource interface and provides attributes of a BMS instance
type BMSResource struct {
	Project    string
	InstanceID string
	Location   string
}

func (BMSResource) GetType() string {
	return "bms"
}

// The data provider interface for BMS environment
type bmsDataProvider interface {
	getProject() string
	getLocation() string
	getInstanceID() string
}

type BMSResourceBuilderInterface interface {
	GetResource() (Resource, error)
}

type BMSResourceBuilder struct {
	provider bmsDataProvider
}

// List of single-valued attributes (non-nested)
var singleBMSAttributeSpec = map[gceAttribute]func(bmsDataProvider) string{
	project:    bmsDataProvider.getProject,
	location:   bmsDataProvider.getLocation,
	instanceID: bmsDataProvider.getInstanceID,
}

// Return a resource instance with all the attributes
// based on the single and nested attributes spec
func (gd *BMSResourceBuilder) GetResource() (Resource, error) {
	singleAttributes := map[gceAttribute]string{}
	for attrName, attrGetter := range singleBMSAttributeSpec {
		singleAttributes[attrName] = attrGetter(gd.provider)
	}

	res := BMSResource{
		Project:    singleAttributes[project],
		Location:   singleAttributes[location],
		InstanceID: singleAttributes[instanceID],
	}
	return res, nil
}
