package bundlelocality

import (
	"testing"
)

func init() {
	initUpstreamClient()
}

func TestContBund(t *testing.T) {
	bundInfo := GetContainerBundles("yolo11:latest")

	if len(bundInfo) == 0 {
		t.Logf("No bundles found for the container.")
		return
	}

	// Bundle: python, Version: 3.11-slim, Size: 0
	// Bundle: numpy, Version: >=1.23.0, Size: 0
	// Bundle: psutil, Version: any, Size: 0

	for _, b := range bundInfo {
		t.Logf("Bundle: %s, Specifier: %s, Size: %v", b.name, b.specifier, b.size)
	}
}
