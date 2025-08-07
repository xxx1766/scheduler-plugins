package layerlocality

import "testing"

func TestLayerPrefabs(t *testing.T) {
	rp := GetContainerLayers("example.com/myimage:tagver")
	for _, r := range rp {
		t.Logf("%v %v %v %v", r.Name, r.SpecType, r.Size, r.Specifier)
	}
}
