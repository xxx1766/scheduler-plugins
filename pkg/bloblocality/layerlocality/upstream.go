package layerlocality

import (
	// "path/filepath"
	// "encoding/json"
	// "os"
	"strings"
	// "k8s.io/klog/v2"
	// "github.com/L-F-Z/TaskC/pkg/prefabservice"
)

// var svcClient *prefabservice.PrefabService

func initUpstreamClient() {
	// we need a real docker / cri-o client
	/*
		homeDir, err := os.UserHomeDir() // '/root' (for example)
		if err != nil {
			klog.Fatal(err)
		}
		workDir := filepath.Join(homeDir, "staging", "upstream")
		err = os.MkdirAll(workDir, 0755)
		if err != nil {
			klog.Fatal(err)
		}

		ps, err := prefabservice.NewUserService(workDir, upstramSvc)
		if err != nil {
			klog.Fatalf("[Layer Locality] Failed to create PrefabService: %v", err)
		}
		svcClient = ps
	*/
}

func splitNormalizedLayerNameAndTag(normalizedName string) (name string, tag string) {
	lastColonIndex := strings.LastIndex(normalizedName, ":")

	if lastColonIndex == -1 {
		return normalizedName, "latest"
	}

	name = normalizedName[:lastColonIndex]
	tag = normalizedName[lastColonIndex+1:]

	return name, tag
}

/* func requestVirtualImageManifest(name, tag string) MiniImageManifest {
	// tag is not used in virtual query
	refreshManifestStore()
	im, ok := virtManifestStore[name]
	if !ok {
		return MiniImageManifest{LayersData: nil}
	}
	return im
}

func RequestImageManifest(name, tag string) MiniImageManifest {
	im := requestVirtualImageManifest(name, tag)
	return im
} */

// GetContainerLayers returns all layers a container required.
func GetContainerLayers(nameTag string) []RemotePrefabInfo {
	retList := []RemotePrefabInfo{}
	name, tag := splitNormalizedLayerNameAndTag(nameTag)

	// bp, err := svcClient.RequestClosureBlueprint(name, tag)
	/* im := RequestImageManifest(name, tag)
	if im.LayersData != nil {
		klog.Errorf("[Layer Locality] Failed to request image manifest for %s:%s", name, tag)
		return retList
	} */

	retList = append(retList, RemotePrefabInfo{
		SpecType:  "Closure",
		Name:      name,
		Specifier: tag,
		Size:      0., // Size is not used in this context
	})

	/* for _, layer := range im.LayersData {
		// for _, p := range layer {
		// klog.Infof("[Layer Locality] [Image=%s:%s] layer: %s, version: %s, size: 0", name, tag, p.Name, p.Specifier)
		layerInfo := RemotePrefabInfo{
			// SpecType:  p.SpecType, // e.g., "image", "package", etc.
			Name: layer.Digest,
			// Specifier: p.Specifier,
			Size: float64(layer.Size), // Size is not used in this context
		}
		retList = append(retList, layerInfo)
		// }
	} */

	return retList
}
