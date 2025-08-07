package layerlocality

import "testing"

func TestQueryLayers(t *testing.T) {
	t.Logf("size: %v\n", QueryNodeLayers("127.0.0.1", GetContainerLayers(normalizedImageName("11.0.1.37:9988/goharbor/testimg3:latest"))))
}
