package resourcedetector

import (
	gcp_metadata "cloud.google.com/go/compute/metadata"
)

type Resource interface {
	GetType() string
}

// Get a resource instance for the current environment, which can be used
// to get all the attributes
func GetResource() (Resource, error) {
	switch {
	case gcp_metadata.OnGCE():
		return GetGCEResource()
	default:
		return GetUnrecognizedPlatformResource()
	}
}

// UnrecognizedPlatformResource that returns an empty resource instance without any attributes
// for unrecognized environments
type UnrecognizedPlatformResource struct {
}

func (UnrecognizedPlatformResource) GetType() string {
	return "unrecognized platform"
}

func GetUnrecognizedPlatformResource() (Resource, error) {
	return UnrecognizedPlatformResource{}, nil
}
