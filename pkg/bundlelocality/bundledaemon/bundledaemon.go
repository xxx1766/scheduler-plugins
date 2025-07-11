package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/TaskC/pkg/prefabservice"
)

const (
	endPort    string = "9998"
	upstramSvc string = "https://prefab.cs.ac.cn:10062"
	workDir    string = "/var/lib/taskc"
)

var bm *bundle.BundleManager

// refer to `TaskC/pkg/prefab/prefab.go` `type Prefab struct`
type RemotePrefabInfo struct {
	SpecType  string  `json:"spectype"` // e.g., "image", "package", etc.
	Name      string  `json:"name"`
	Specifier string  `json:"specifier"` // e.g., "v1.0.0", "latest", etc.
	Size      float64 `json:"size"`      // in MiB
}

type LocalBundleInfo struct {
	id      string
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

func GetLocalFileSize(id string) (int64, error) {
	url := fmt.Sprintf("%s/file?id=%s", upstramSvc, id)

	client := &http.Client{}

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status Code %d", resp.StatusCode)
	}

	lengthStr := resp.Header.Get("Content-Length")
	if lengthStr == "" {
		return 0, fmt.Errorf("content-Length Not Found")
	}

	contentLength, err := strconv.ParseInt(lengthStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return contentLength, nil
}

func CompareAndCalculate(nodeIP string, l map[string][]LocalBundleInfo, r []RemotePrefabInfo) float64 {
	// klog.Infof("Query Local TaskC IP: %v", nodeIP)
	sizes := 0.0

	if len(r) == 0 {
		klog.Warningf("[Bundle Daemon] nodeIP=%v, No Remote Prefabs Found.", nodeIP)
		return 0.0
	}

	thisBundleSize := 0.0

	for _, b := range r { // compare a remote prefab with local bundles
		thisBundleSize = 0.0

		b_specT, b_name, b_ver := b.SpecType, b.Name, b.Specifier
		// klog.Infof("[Bundle Daemon] nodeIP=%v, Checking Remote Bundle: [%v]{%v}:(%v)", nodeIP, b_specT, b_name, b_ver)

		v, ok := l[b_name]

		if !ok {
			// klog.Infof("[Bundle Daemon] nodeIP=%v, No Local Bundle Found for [%s]%s:%s", nodeIP, b_specT, b_name, b_ver)
			continue
		}

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

		// klog.Infof("[Bundle Daemon] Found Local Bundle: %s (%s)\n", name, version)

		id, exists := bm.GetBundleID(name, version) // ensure the bundle exists in the BundleManager

		if exists {
			size, err := GetLocalFileSize(id)
			if err != nil {
				size = 1 // default size if the file size cannot be determined
			}
			localBundleDict[name] = append(localBundleDict[name], LocalBundleInfo{
				id:      id, // id is not used in this context, can be set later if needed
				name:    name,
				version: version,
				size:    float64(size), // in MiB
			})
		}
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

	/* for _, b := range remotePrefabs {
		klog.Infof("[Bundle Daemon] Remote Bundle: %s, Type: %s, Version: %s, Size: %.2f MiB", b.Name, b.SpecType, b.Specifier, b.Size)
	} */

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
