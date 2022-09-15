package resourcedetector

import (
	gcp_metadata "cloud.google.com/go/compute/metadata"
)

type Detector interface {
	GetType() string
}

// Get a Detector for the current environment, which can be used
// to get all the attributes
func GetDetector() (Detector, error) {
	switch {
	case gcp_metadata.OnGCE():
		return GetGCEDetector()
	default:
		return GetUnrecognizedPlatformDetector()
	}
}

// UnrecognizedPlatformDetector that returns an empty detector without any attributes
// for unrecognized environments
type UnrecognizedPlatformDetector struct {
}

func (UnrecognizedPlatformDetector) GetType() string {
	return "unrecognized platform"
}

func GetUnrecognizedPlatformDetector() (Detector, error) {
	return UnrecognizedPlatformDetector{}, nil
}
