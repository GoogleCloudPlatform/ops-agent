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

import (
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp"
)

func TestGettingResourceWithoutError(t *testing.T) {
	fakeProvider := FakeProvider{key: "some_key", value: "some_value"}
	fakeBuilder := GCEResourceBuilder{&fakeProvider}
	actual, err := fakeBuilder.GetResource()
	if err != nil {
		t.Errorf("should not have error")
	}
	if actual == nil {
		t.Errorf("should not return nil resource when no error")
	} else if r, ok := actual.(GCEResource); ok {
		if r.InstanceID != "some_value" {
			t.Errorf("resource attribute InstanceID has wrong value")
		}
		if r.InterfaceIPv4["some_key"] != "some_value" {
			t.Errorf("resource attribute InterfaceIPv4 has wrong value")
		}
		if r.DefaultScopes[0] != "some_value" {
			t.Errorf("resource attribute DefaultScopes has wrong value")
		}
	} else {
		t.Errorf("should have created GCEResource")
	}
}

func TestErrorGettingResource(t *testing.T) {
	fakeProvider := FakeProvider{err: fmt.Errorf("some error")}
	fakeBuilder := GCEResourceBuilder{&fakeProvider}
	actual, err := fakeBuilder.GetResource()
	if err == nil {
		t.Errorf("should have error")
	}
	if err.Error() != "some error" {
		t.Errorf("should have return the correct error message")
	}
	if actual != nil {
		t.Errorf("should return nil resource when having error")
	}
}

type FakeProvider struct {
	err   error
	key   string
	value string
}

func (fp *FakeProvider) get() (string, error) {
	if fp.err != nil {
		return "", fp.err
	}
	return fp.value, nil
}

func (fp *FakeProvider) getSlice() ([]string, error) {
	if fp.err != nil {
		return []string{}, fp.err
	}
	return []string{fp.value}, nil
}

func (fp *FakeProvider) getMap() (map[string]string, error) {
	if fp.err != nil {
		return map[string]string{}, fp.err
	}
	return map[string]string{fp.key: fp.value}, nil
}

func (fp *FakeProvider) getProject() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getZone() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getNetwork() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getSubnetwork() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getPublicIP() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getPrivateIP() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getInstanceID() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getInstanceName() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getTags() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getMachineType() (string, error) {
	return fp.get()
}

func (fp *FakeProvider) getDefaultScopes() ([]string, error) {
	return fp.getSlice()
}

func (fp *FakeProvider) getMetadata() (map[string]string, error) {
	return fp.getMap()
}

func (fp *FakeProvider) getLabels() (map[string]string, error) {
	return fp.getMap()
}

func (fp *FakeProvider) getInterfaceIPv4s() (map[string]string, error) {
	return fp.getMap()
}

func (fp *FakeProvider) getMIG() (gcp.ManagedInstanceGroup, error) {
	return gcp.ManagedInstanceGroup{}, nil
}
