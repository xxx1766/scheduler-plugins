package main

import (
	"testing"

	"github.com/L-F-Z/TaskC/pkg/bundle"
)

const REPO_PYPI = "PyPI"
const REPO_DOCKERHUB = "DockerHub"
const REPO_APT = "Apt"
const REPO_HUGGINGFACE = "HuggingFace"
const REPO_PREFAB = "Prefab"
const REPO_CLOSURE = "Closure"
const REPO_K8S = "k8s"

func TestSizeFileJSON(t *testing.T) {
	// Test 1
	size, err := GetPakSizeFileJSON("c394e36c-8327-42a7-8a78-a1e090cd7276")
	if err != nil {
		t.Errorf("Failed to get remote file size: %v", err)
		return
	}
	if size <= 0 {
		t.Errorf("Expected size to be greater than 0, got %d", size)
		return
	}
	t.Logf("Remote file size: %d bytes", size) // 395738

	// Test 2
	size, err = GetPakSizeFileJSON("0")
	if err == nil {
		t.Errorf("Expected error for invalid ID, got size %d", size)
		return
	}
	t.Logf("Expected error for invalid ID: %v", err)
}

func TestGetSizesHTTP(t *testing.T) {
	// Test 1
	size, err := GetPakSizeHTTP("001d28b8-076b-4c0b-9a95-ecedf425d148")
	// size, err := GetPakSizeHTTP("dd6b348a-b185-4ff2-9457-b668cbadb0d6")
	if err != nil {
		t.Errorf("Failed to get remote file size: %v", err)
		return
	}
	if size <= 0 {
		t.Errorf("Expected size to be greater than 0, got %d", size)
		return
	}
	t.Logf("Remote file size: %d bytes", size) // 395738

	// Test 2
	size, err = GetPakSizeHTTP("0")
	if err == nil {
		t.Errorf("Expected error for invalid ID, got size %d", size)
		return
	}
	t.Logf("Expected error for invalid ID: %v", err)
}

func TestGetID(t *testing.T) {
	bm, _ := bundle.NewBundleManager(workDir, upstramSvc)

	id, eixsts := bm.GetBundleID("yolo11", "latest")
	if !eixsts {
		t.Errorf("Expected bundle yolo11:latest to exist, but it does not")
		return
	}
	if id == "" {
		t.Errorf("Expected non-empty ID for bundle yolo11:latest, got empty string")
		return
	}
	t.Logf("Bundle ID for yolo11:latest is %s", id) // e3831e62-37ef-4a6c-a686-fe69fa3bdf0c

	_, eixsts = bm.GetBundleID("nonexistent", "latest")
	if eixsts {
		t.Errorf("Expected bundle nonexistent:latest to not exist, but it does")
		return
	}
	t.Logf("Bundle nonexistent:latest does not exist as expected")

	id, eixsts = bm.GetBundleID("registry.k8s.io/kube-scheduler", "v1.32.6")
	if !eixsts {
		t.Errorf("Expected bundle to exist, but it does not")
		return
	}
	if id == "" {
		t.Errorf("Expected non-empty ID for bundle, got empty string")
		return
	}
	t.Logf("Bundle ID for the bundle is %s", id) // e3831e62-37ef-4a6c-a686-fe69fa3bdf0c
}

func TestVerMatch(t *testing.T) {
	if !VersionMatch(REPO_PYPI, "numpy", ">=1.23.0", "1.23.5") {
		t.Errorf("Expected version match for numpy >=1.23.0 with 1.23.5")
	}

	if VersionMatch(REPO_PYPI, "numpy", ">=1.23.0", "1.22.5") {
		t.Errorf("Expected no version match for numpy >=1.23.0 with 1.22.5")
	}

	if !VersionMatch(REPO_DOCKERHUB, "python", "3.11-slim", "3.11-slim") {
		t.Errorf("Expected version match for python 3.11-slim with 3.11-slim")
	}

	if VersionMatch(REPO_DOCKERHUB, "python", "3.11-slim", "3.10-slim") {
		t.Errorf("Expected no version match for python 3.11-slim with 3.10-slim")
	}
}

func TestCompareAndCalculateJSON(t *testing.T) {
	CompareAndCalculateJSON(apps["sam2"])
}

func TestComp(t *testing.T) {
	l := map[string][]LocalBundleInfo{
		"yolo11": {
			{
				name:    "yolo11",
				version: "1.0.0",
				size:    9.0,
			},
			{
				name:    "yolo11",
				version: "2.0.0",
				size:    99.0,
			},
			{
				name:    "yolo11",
				version: "2.1.0",
				size:    999999.0,
			},
		},
		"yolo12": {
			{
				name:    "yolo12",
				version: "3.0.0",
				size:    999.0,
			},
		},
	}

	r := []RemotePrefabInfo{
		{
			SpecType:  REPO_PYPI,
			Name:      "yolo11",
			Specifier: ">=1.5.0",
			Size:      9999.0,
		},
		{
			SpecType:  REPO_DOCKERHUB,
			Name:      "python",
			Specifier: "3.11-slim",
			Size:      99999.0,
		},
	}

	t.Logf("size = %f\n", CompareAndCalculate("192.168.1.1", l, r))
}
