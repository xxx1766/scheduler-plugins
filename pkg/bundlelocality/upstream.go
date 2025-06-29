package bundlelocality

import (
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"

	"github.com/L-F-Z/TaskC/pkg/prefabservice"
)

var svcClient *prefabservice.PrefabService

func initUpstreamClient() {
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
		klog.Fatalf("[Bundle Locality] Failed to create PrefabService: %v", err)
	}
	svcClient = ps
}

func splitNormalizedBundleNameAndTag(normalizedName string) (name string, tag string) {
	lastColonIndex := strings.LastIndex(normalizedName, ":")

	if lastColonIndex == -1 {
		return normalizedName, "latest"
	}

	name = normalizedName[:lastColonIndex]
	tag = normalizedName[lastColonIndex+1:]

	return name, tag
}

// GetContainerBundles returns all bundles a container required.
func GetContainerBundles(nameTag string) []RemotePrefabInfo {
	retList := []RemotePrefabInfo{}
	name, tag := splitNormalizedBundleNameAndTag(nameTag)

	bp, err := svcClient.RequestClosureBlueprint(name, tag)
	if err != nil {
		klog.Errorf("[Bundle Locality] Failed to request closure blueprint for %s:%s: %v", name, tag, err)
		return retList
	}

	for _, prefab := range bp.Depend {
		for _, p := range prefab {
			// klog.Infof("[Bundle Locality] [Image=%s:%s] bundle: %s, version: %s, size: 0", name, tag, p.Name, p.Specifier)
			bundleInfo := RemotePrefabInfo{
				SpecType:  p.SpecType, // e.g., "image", "package", etc.
				Name:      p.Name,
				Specifier: p.Specifier,
				Size:      1., // Size is not used in this context
			}
			retList = append(retList, bundleInfo)
		}
	}

	return retList
}
