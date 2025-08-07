package bundlelocality

import (
	"testing"
)

func init() {
	initUpstreamClient()
}

func TestNameSplitter(t *testing.T) {
	name, tag := splitNormalizedBundleNameAndTag("yolo11:latest")
	if name != "yolo11" || tag != "latest" {
		t.Errorf("Expected 'yolo11' and 'latest', got '%s' and '%s'", name, tag)
	}

	name, tag = splitNormalizedBundleNameAndTag("yolo11")
	if name != "yolo11" || tag != "latest" {
		t.Errorf("Expected 'yolo11' and 'latest', got '%s' and '%s'", name, tag)
	}
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
		t.Logf("Bundle: %s, Specifier: %s, Size: %v", b.Name, b.Specifier, b.Size)
	}
}
