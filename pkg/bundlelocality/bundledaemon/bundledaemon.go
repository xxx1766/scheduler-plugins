package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"k8s.io/klog/v2"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/TaskC/pkg/prefabservice"
)

const (
	endPort    string = "9998"
	upstramSvc string = "https://prefab.cs.ac.cn:10062"
)

var bm *bundle.BundleManager

// refer to `TaskC/pkg/prefab/prefab.go` `type Prefab struct`
type RemotePrefabInfo struct {
	specType  string // e.g., "image", "package", etc.
	name      string
	specifier string
	size      float64 // in MiB (currently 1.0, which is NOT my fault)
}

type LocalBundleInfo struct {
	name    string
	version string
	size    float64 // in MiB
}

// localVer is "single" and acceptableVer is a range
func VersionMatch(specType string, name string, specifier string, version string) bool {
	decodedSpecifier, err1 := prefabservice.DecodeAnySpecifier(specType, specifier)
	parsedVersion, err2 := prefabservice.ParseAnyVersion(specType, version)

	if err1 != nil || err2 != nil {
		return false
	}

	return decodedSpecifier.Contains(parsedVersion)
}

func CompareAndCalculate(nodeIP string, l map[string][]LocalBundleInfo, r []RemotePrefabInfo) float64 {
	klog.Infof("Query Local TaskC IP: %v", nodeIP)
	sizes := 0.0

	if len(r) == 0 {
		klog.Warningf("[Bundle Daemon] nodeIP=%v, No Remote Prefabs Found.", nodeIP)
		return 0.0
	}

	for _, b := range r { // compare a remote prefab with local bundles
		b_specT, b_name, b_ver := b.specType, b.name, b.specifier

		v, ok := l[b_name]

		if !ok {
			klog.Infof("[Bundle Daemon] nodeIP=%v, No Local Bundle Found for [%s]%s:%s", nodeIP, b_specT, b_name, b_ver)
			continue
		}

		thisBundleSize := 1.0

		for _, localBundle := range v {
			// klog.Infof("[Bundle Daemon] nodeIP=%v, Found Local Bundle: %s (%s), size: %.2f MiB", nodeIP, localBundle.name, localBundle.version, localBundle.size)

			// Check if the local bundle version matches the remote prefab specifier
			if VersionMatch(b_specT, localBundle.name, b_ver, localBundle.version) {
				thisBundleSize = max(thisBundleSize, localBundle.size)
				// klog.Infof("[Bundle Daemon] nodeIP=%v, Matched Local Bundle: %s (%s), size: %.2f MiB", nodeIP, localBundle.name, localBundle.version, localBundle.size)
			} else {
				// klog.Infof("[Bundle Daemon] nodeIP=%v, No Match for Local Bundle: %s (%s) with Remote Prefab Specifier: %s", nodeIP, localBundle.name, localBundle.version, b_ver)
			}
		}

		sizes += thisBundleSize
	}

	return sizes
}

func ListLocalBundles() map[string][]LocalBundleInfo {
	// get local bundles
	nameVersions := bm.ListNames() // in the format of `name (version)`

	var localBundleDict = make(map[string][]LocalBundleInfo)
	for _, nameVersion := range nameVersions {
		// nameVersion is in the format "name (version)"
		lastOpen := strings.LastIndex(nameVersion, "(")
		lastClose := strings.LastIndex(nameVersion, ")")

		if lastOpen == -1 || lastClose == -1 || lastClose < lastOpen {
			klog.Warningf("[Bundle Daemon] Bundle %s does not have a valid version format.", nameVersion)
			continue
		}

		name := strings.TrimSpace(nameVersion[:lastOpen])
		version := strings.TrimSpace(nameVersion[lastOpen+1 : lastClose])
		// fmt.Printf("[Bundle Daemon] Found Local Bundle: %s (%s)\n", name, version)

		localBundleDict[name] = append(localBundleDict[name], LocalBundleInfo{
			name:    name,
			version: version,
			size:    1., // in MiB (currently 1.0, which is NOT my fault)
		})
	}

	return localBundleDict
}

func bundleHandler(w http.ResponseWriter, r *http.Request) {
	/* query := r.URL.Query()
	bundleName := query.Get("name")
	bundleVersion := query.Get("version")

	if bundleName == "" || bundleVersion == "" {
		http.Error(w, "missing 'name' or 'version' query param", http.StatusBadRequest)
		return
	} */

	if r.Method != "POST" {
		http.Error(w, "[Bundle Daemon] method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		nodeIP = host
	}

	var remotePrefabs []RemotePrefabInfo
	err := json.NewDecoder(r.Body).Decode(&remotePrefabs)
	if err != nil {
		http.Error(w, "[Bundle Daemon] invalid JSON payload", http.StatusBadRequest)
		return
	}

	sizes := CompareAndCalculate(nodeIP, ListLocalBundles(), remotePrefabs)
	var response struct {
		sizes float64
	}
	response.sizes = sizes

	w.Header().Set("Content-Type", "application/json")
	resultBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(resultBytes)
}

func main() {
	klog.InitFlags(nil)
	var err error

	workDir := "/var/lib/taskc"
	bm, err = bundle.NewBundleManager(workDir, upstramSvc)
	if err != nil {
		klog.Fatalf("[Bundle Daemon] Failed to create BundleManager: %v", err)
	}

	http.HandleFunc("/bundles", bundleHandler)

	klog.Info(fmt.Sprintf("[Bundle Daemon] Starting HTTP Server on :%s", endPort))
	err = http.ListenAndServe(fmt.Sprintf(":%s", endPort), nil)
	if err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}
