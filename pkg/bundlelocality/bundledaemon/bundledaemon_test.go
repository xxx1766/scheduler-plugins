package main

import (
	"testing"
)

const REPO_PYPI = "PyPI"
const REPO_DOCKERHUB = "DockerHub"
const REPO_APT = "Apt"
const REPO_HUGGINGFACE = "HuggingFace"
const REPO_PREFAB = "Prefab"
const REPO_CLOSURE = "Closure"
const REPO_K8S = "k8s"

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
