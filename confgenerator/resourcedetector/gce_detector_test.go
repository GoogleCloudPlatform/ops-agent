package resourcedetector

import (
	"fmt"
	"testing"
)

func TestGettingDetectorWithoutError(t *testing.T) {
	fakeProvider := FakeProvider{key: "some_key", value: "some_value"}
	fakeBuilder := GCEDetectorBuilder{&fakeProvider}
	actual, err := fakeBuilder.GetDetector()
	if err != nil {
		t.Errorf("should not have error")
	}
	if actual == nil {
		t.Errorf("should not return nil detector when no error")
	} else if d, ok := actual.(GCEDetector); ok {
		if d.InstanceID != "some_value" {
			t.Errorf("detector attribute InstanceID has wrong value")
		}
		if d.InterfaceIPv4["some_key"] != "some_value" {
			t.Errorf("detector attribute InterfaceIPv4 has wrong value")
		}
	} else {
		t.Errorf("should have created GCEDetector")
	}
}

func TestErrorGettingDetector(t *testing.T) {
	fakeProvider := FakeProvider{err: fmt.Errorf("some error")}
	fakeBuilder := GCEDetectorBuilder{&fakeProvider}
	actual, err := fakeBuilder.GetDetector()
	if err == nil {
		t.Errorf("should have error")
	}
	if err.Error() != "some error" {
		t.Errorf("should have return the correct error message")
	}
	if actual != nil {
		t.Errorf("should return nil detector when having error")
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

func (fp *FakeProvider) getMetadata() (map[string]string, error) {
	return fp.getMap()
}

func (fp *FakeProvider) getLabels() (map[string]string, error) {
	return fp.getMap()
}

func (fp *FakeProvider) getInterfaceIPv4s() (map[string]string, error) {
	return fp.getMap()
}
