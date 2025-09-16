package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/L-F-Z/TaskC/pkg/bundle"
	"github.com/L-F-Z/TaskC/pkg/prefabservice"
)

const (
	endPort     string = "9998"
	// upstramSvc  string = "http://9.0.3.34:9999"
	upstramSvc  string = "https://prefab.cs.ac.cn:10062"
	workDir     string = "/root/simulating"
	payloadJSON string = "payload.json"
	appJSON     string = "apps.json"
	infoJSON    string = "/PrefabService/File.json"
	bundleJSON  string = "/bundles.json"
	contRuntime string = "cri-o"
)

type LayerData struct {
	Digest string `json:"Digest"` // e.g., "sha256:1234567890abcdef..."
	Size   int64  `json:"Size"`   // in bytes
}


type MiniImageManifest struct {
	LayersData []LayerData `json:"LayersData"`
	Layers     []string    `json:"Layers"`
}

type JSONPakInfo struct {
	Filename string `json:"filename"`
	Filetype string `json:"filetype"`
	Filesize int    `json:"filesize"`
}

// refer to `TaskC/pkg/prefab/prefab.go` `type Prefab struct`
type RemotePrefabInfo struct {
	SpecType  string  `json:"spectype"` // e.g., "image", "package", etc.
	Name      string  `json:"name"`
	Specifier string  `json:"specifier"` // e.g., "v1.0.0", "latest", etc.
	Size      float64 `json:"size"`      // in MiB
}

type AppEntries struct {
	TaskC   Entry   `json:"taskc"`
	Prefabs []Entry `json:"prefabs"`
}

type Entry struct {
	PrefabID    string `json:"prefabID"`
	BlueprintID string `json:"blueprintID"`
	PrefabSize  uint64 `json:"prefabSize"`
}

type LocalBundleInfo struct {
	id      string
	name    string
	version string
	size    float64 // in B
}

type crictlImage struct {
	RepoTags []string `json:"repoTags"`
}

type crictlImagesResponse struct {
	Images []crictlImage `json:"images"`
}

var apps map[string]AppEntries
var bm *bundle.BundleManager
var bms = make(map[string]*bundle.BundleManager) 
// var packageMap = make(map[string]JSONPakInfo)
var packageMaps = make(map[string]map[string]JSONPakInfo)
var bundleMaps = make(map[string]map[string]LocalBundleInfo)
var mapMutex = &sync.RWMutex{}
var virtManifestStore map[string]MiniImageManifest
var localManifest map[string]MiniImageManifest

func init() {
	data, err := os.ReadFile(appJSON)
	if err != nil {
		klog.Errorf("Failed to read apps.json: %v", err)
	}
	if err := json.Unmarshal(data, &apps); err != nil {
		klog.Errorf("Failed to parse apps.json: %v", err)
	}
	ReloadFileJSON()
	ReloadPayloadJSON()
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

func ReloadFileJSON() error {
	mapMutex.Lock()
	defer mapMutex.Unlock()

	packageMaps = make(map[string]map[string]JSONPakInfo)

	for idx := 1; idx <= 1000; idx++ {
		folderName := fmt.Sprintf("/10.0.%d.%d", idx/250, idx%250+1)
		filePath := filepath.Join(workDir+folderName, infoJSON)

		file, err := os.Open(filePath)
		if err != nil {
			klog.Warningf("failed to open %s: %v", filePath, err)
			continue
		}
		var tempMap map[string]JSONPakInfo
		decoder := json.NewDecoder(file)
		err = decoder.Decode(&tempMap)
		file.Close()
		if err != nil {
			klog.Warningf("failed to decode %s: %v", filePath, err)
			continue
		}
		packageMaps[folderName] = tempMap

		bundleFilePath := filepath.Join(workDir+folderName, bundleJSON)
		bundleFile, err := os.Open(bundleFilePath)
		if err != nil {
			klog.Warningf("failed to open %s: %v", bundleFilePath, err)
			continue
		}
		var tempBundleMap map[string]LocalBundleInfo
		decoder = json.NewDecoder(bundleFile)
		err = decoder.Decode(&tempBundleMap)
		bundleFile.Close()
		if err != nil {
			klog.Warningf("failed to decode %s: %v", bundleFilePath, err)
			continue
		}
		bundleMaps[folderName] = tempBundleMap
	}

	return nil
	
}

func ReloadFileJSONFromNodeIP(nodeIP string) error {
	filePath := filepath.Join(workDir+"/"+nodeIP, infoJSON)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open info.json: %v", err)
	}
	defer file.Close()
	
	var tempMap map[string]JSONPakInfo
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&tempMap)
	if err != nil {
		return fmt.Errorf("failed to decode info.json: %v", err)
	}
	packageMaps[nodeIP] = tempMap

	bundleFilePath := filepath.Join(workDir+"/"+nodeIP, bundleJSON)
	bundleFile, err := os.Open(bundleFilePath)
	if err != nil {
		return fmt.Errorf("failed to open bundles.json: %v", err)
	}
	defer bundleFile.Close()
	var tempBundleMap map[string]LocalBundleInfo
	decoder = json.NewDecoder(bundleFile)
	err = decoder.Decode(&tempBundleMap)
	if err != nil {
		return fmt.Errorf("failed to decode bundles.json: %v", err)
	}
	bundleMaps[nodeIP] = tempBundleMap

	return nil
}

func ReloadPayloadJSON() {
	jsonData, err := os.ReadFile(payloadJSON)
	if err != nil {
		return
	}

	var manifests map[string]MiniImageManifest
	err = json.Unmarshal(jsonData, &manifests)
	if err != nil {
		return
	}

	virtManifestStore = manifests
}

func GetPakSizeHTTP(id string) (int64, error) {
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

func CompareAndCalculateJSON(appE AppEntries, nodeIP string) float64 {
	sizeInBytes := 0
	pkgmap := GetPulledPrefabsFromFile(nodeIP)
	if  pkgmap == nil || len(pkgmap) == 0 {
    	sizeInBytes = .0
	}else{
		for _, e := range appE.Prefabs {
			if e.PrefabID != "" {
				if _, exists := pkgmap[e.PrefabID]; exists {
					sizeInBytes += int(e.PrefabSize)
				} else {
					// klog.Warningf("[Bundle Daemon] Prefab ID %s not found in info.json", e.PrefabID)
				}
			}
		}
	}
	// fmt.Printf("[Bundle Daemon] Total size in bytes: %d B, in megabytes: %.f MiB\n", sizeInBytes, float64(sizeInBytes)

	return float64(sizeInBytes) // Convert bytes to MiB
}

func CompareAndCalculate(nodeIP string, l map[string][]LocalBundleInfo, r []RemotePrefabInfo) float64 {
	klog.Infof("CompareAndCalculate: %v", nodeIP)
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

func ListLocalBundles(nodeIP string) map[string][]LocalBundleInfo {
	// get local bundles
	// nameVersions := bms[nodeIP].ListNames() // in the format of `name (version)`
	var localBundleDict = make(map[string][]LocalBundleInfo)
	
	// // From real situation
	// for _, nameVersion := range nameVersions {
	// 	// nameVersion is in the format "name (version)"
	// 	lastOpen := strings.LastIndex(nameVersion, "(")
	// 	lastClose := strings.LastIndex(nameVersion, ")")

	// 	if lastOpen == -1 || lastClose == -1 || lastClose < lastOpen {
	// 		klog.Warningf("[Bundle Daemon] Bundle %s does not have a valid version format.", nameVersion)
	// 		continue
	// 	}

	// 	name := strings.TrimSpace(nameVersion[:lastOpen])
	// 	version := strings.TrimSpace(nameVersion[lastOpen+1 : lastClose])

	// 	// klog.Infof("[Bundle Daemon] Found Local Bundle: %s (%s)\n", name, version)

	// 	id, exists := bms[nodeIP].GetBundleID(name, version) // ensure the bundle exists in the BundleManager

	// 	if exists {
	// 		size, err := GetPakSizeHTTP(id)
	// 		if err != nil {
	// 			size = 1 // default size if the file size cannot be determined
	// 		}
	// 		localBundleDict[name] = append(localBundleDict[name], LocalBundleInfo{
	// 			id:      id, // id is not used in this context, can be set later if needed
	// 			name:    name,
	// 			version: version,
	// 			size:    float64(size), // in MiB
	// 		})
	// 	}
	// }

	// From simulated situation
	// ReloadFileJSONFromNodeIP(nodeIP)
	bundlelist := GetPulledBundleNamesFromFile(nodeIP)
	if bundlelist == nil {
		klog.Warningf("[Bundle Daemon] nodeIP=%v, No Local Bundle Info Found.", nodeIP)
		return localBundleDict
	}
	
	for _, bundle := range bundlelist {
		localBundleDict[bundle.name] = append(localBundleDict[bundle.name], bundle)
	}
	// if bundleMap, exists := bundleMaps[nodeIP]; exists {
	// 	for _, bundle := range bundleMap {
	// 		localBundleDict[bundle.name] = append(localBundleDict[bundle.name], bundle)
	// 	}
	// } else {
	// 	klog.Warningf("[Bundle Daemon] nodeIP=%v, No Local Bundle Info Found.", nodeIP)
	// }

	return localBundleDict
}

func GetPulledImageNames(runtime string) map[string][]string {
	imageMap := make(map[string][]string)

	if runtime == "cri-o" {
		cmd := exec.Command("crictl", "images", "--output", "json")
		output, err := cmd.Output()

		if err != nil {
			fmt.Printf("Error executing command: %v\n", err)
			output, err = os.ReadFile("crictl_images.json")
			if err != nil {
				return nil
			}
		}

		var response crictlImagesResponse
		if err := json.Unmarshal(output, &response); err != nil {
			fmt.Printf("Error parsing JSON: %v\n", err)
			return nil
		}

		for _, image := range response.Images {
			for _, repoTag := range image.RepoTags {
				parts := strings.Split(repoTag, ":")
				if len(parts) < 2 {
					continue
				}

				fullName := strings.Join(parts[:len(parts)-1], ":")

				name := fullName
				if lastSlash := strings.LastIndex(fullName, "/"); lastSlash != -1 {
					name = fullName[lastSlash+1:]
				}

				tag := parts[len(parts)-1]

				imageMap[name] = append(imageMap[name], tag)
			}
		}
	}

	return imageMap
}

func GetPulledBundleNamesFromFile(nodeIP string) map[string]LocalBundleInfo {
	bundleMap := make(map[string]LocalBundleInfo)

	filePath := filepath.Join(workDir+"/"+nodeIP, bundleJSON)
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	
	err = json.Unmarshal(file, &bundleMap)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return nil
	}
	
	return bundleMap
}

func GetPulledPrefabsFromFile(nodeIP string) map[string]JSONPakInfo {
	pakMap := make(map[string]JSONPakInfo)

	filePath := filepath.Join(workDir+"/"+nodeIP, infoJSON)
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	
	err = json.Unmarshal(file, &pakMap)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return nil
	}
	
	return pakMap
}


func GetPulledImageNamesFromFile(nodeIP string) map[string][]string {
	imageMap := make(map[string][]string)

	filePath := filepath.Join(workDir+"/"+nodeIP, "/images.json")
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	var response crictlImagesResponse
	if err := json.Unmarshal(file, &response); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return nil
	}
	for _, image := range response.Images {
		for _, repoTag := range image.RepoTags {
			parts := strings.Split(repoTag, ":")
			if len(parts) < 2 {
				continue
			}

			fullName := strings.Join(parts[:len(parts)-1], ":")

			name := fullName
			if lastSlash := strings.LastIndex(fullName, "/"); lastSlash != -1 {
				name = fullName[lastSlash+1:]
			}

			tag := parts[len(parts)-1]

			imageMap[name] = append(imageMap[name], tag)
				
		}
	}

	return imageMap
}

func handleRequest(w http.ResponseWriter, r *http.Request) ([]RemotePrefabInfo, string) {
	/* query := r.URL.Query()
	bundleName := query.Get("name")
	bundleVersion := query.Get("version")

	if bundleName == "" || bundleVersion == "" {
		http.Error(w, "missing 'name' or 'version' query param", http.StatusBadRequest)
		return
	} */

	/* for _, b := range remotePrefabs {
		klog.Infof("[Bundle Daemon] Remote Bundle: %s, Type: %s, Version: %s, Size: %.2f MiB", b.Name, b.SpecType, b.Specifier, b.Size)
	} */
	path := r.URL.Path
	var nodeIP string

	if strings.HasPrefix(path, "/bundles/") {
		nodeIP = strings.TrimPrefix(path, "/bundles/")
	} else if strings.HasPrefix(path, "/layers/") {
		nodeIP = strings.TrimPrefix(path, "/layers/")
	} else {
		http.Error(w, "Invalid path format. Expected /bundles/{nodeIP} or /layers/{nodeIP}", http.StatusBadRequest)
		return nil, ""
	}
	
	if nodeIP == "" {
		http.Error(w, "Node IP is required in path", http.StatusBadRequest)
		return nil, ""
	}
	klog.Infof("[Daemon] Extracted nodeIP from path: %s", nodeIP)

	if r.Method != "POST" {
		http.Error(w, "[Daemon] method not allowed", http.StatusMethodNotAllowed)
		return nil, nodeIP
	}

	var remotePrefabs []RemotePrefabInfo
	err := json.NewDecoder(r.Body).Decode(&remotePrefabs)
	if err != nil {
		klog.Errorf("[Daemon] JSON decode error: %v", err)
		http.Error(w, "[Daemon] invalid JSON payload", http.StatusBadRequest)
		return nil, nodeIP
	}

	klog.Infof("[Daemon] Received %d remote prefabs for node %s", len(remotePrefabs), nodeIP)

	return remotePrefabs, nodeIP
}

func handleReponse(w http.ResponseWriter, r *http.Request, sizes float64) {
	var response struct {
		Sizes float64 `json:"sizes"`
	}
	response.Sizes = sizes

	nodeIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		nodeIP = host
	}

	klog.Infof("[Daemon] nodeIP=%v, Total Size: %.2f B", nodeIP, response.Sizes)

	w.Header().Set("Content-Type", "application/json")
	resultBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(resultBytes)
}

func layerHandlerInner(remotePrefabs []RemotePrefabInfo, nodeIP string) float64 {
	var sizes = .0
	if len(remotePrefabs) == 0 {
        klog.Warningf("[Daemon] No remote prefabs provided for node %s", nodeIP)
        return .0
    }

	// example: `11.0.1.37:9988/goharbor/testimg1`
	fullName := remotePrefabs[0].Name

	name := fullName
	if lastSlash := strings.LastIndex(fullName, "/"); lastSlash != -1 {
		name = fullName[lastSlash+1:]
	}

	im, isFixed := virtManifestStore[name]

	klog.Infof("[Layer Daemon] nodeIP=%v, App: %s, Fixed: %v", nodeIP, remotePrefabs[0].Name, isFixed)

	if !isFixed {
		return .0
	}

	layerMap := make(map[string]float64)
	calcuMap := make(map[string]bool)

	for _, layer := range im.LayersData {
		cleanDigest := strings.TrimPrefix(layer.Digest, "sha256:")
		layerMap[cleanDigest] = float64(layer.Size)
		calcuMap[cleanDigest] = false
	}

	/* for u, v := range layerMap {
		fmt.Printf("[Debug] %v %v\n", u, v)
	} */

	for img := range GetPulledImageNamesFromFile(nodeIP) {
		if im, ok := virtManifestStore[img]; ok {
			for _, layer := range im.Layers {
				cleanDigest := strings.TrimPrefix(layer, "sha256:")
				if size, exists := layerMap[cleanDigest]; exists {
					if !calcuMap[cleanDigest] {
						calcuMap[cleanDigest] = true
						sizes += size
						// klog.Infof("[Debug] %v +%v\n", cleanDigest, size)
					}
				}
			}
		}
	}

	// klog.Infof("[Debug] sizes = %.f MiB\n", sizes)

	return sizes
}

func layerHandler(w http.ResponseWriter, r *http.Request) {
	remotePrefabs, nodeIP := handleRequest(w, r)
	handleReponse(w, r, layerHandlerInner(remotePrefabs, nodeIP))
}

func bundleHandler(w http.ResponseWriter, r *http.Request) {
	remotePrefabs, nodeIP := handleRequest(w, r)

	var sizes = .0
	if len(remotePrefabs) == 0 {
		klog.Warningf("[Bundle Daemon] No remote prefabs provided for node %s", nodeIP)
		handleReponse(w, r, .0)
		return
	}
	app, isFixed := apps[remotePrefabs[0].Name]

	klog.Infof("[Bundle Daemon] nodeIP=%v, App: %s, Fixed: %v", nodeIP, remotePrefabs[0].Name, isFixed)

	// if !isFixed {
	// 	sizes = CompareAndCalculate(nodeIP, ListLocalBundles(nodeIP), remotePrefabs[1:]) // skip the first one which is the closure prefab
	// } else {
	// 	if bundleMaps[nodeIP] == nil {
	// 		klog.Errorf("[Bundle Daemon] nodeIP=%v, Failed to reload info.json", nodeIP)
	// 		sizes = .0
	// 	} else {
	// 		sizes = CompareAndCalculateJSON(app, nodeIP)
	// 	}
	// }
	if !isFixed {
		sizes =  .0
	}else {
		// if ReloadFileJSONFromNodeIP(nodeIP) != nil {
		// 	klog.Errorf("[Bundle Daemon] nodeIP=%v, Failed to reload info.json", nodeIP)
		// 	sizes = .0
		// }else {
		// 	sizes = CompareAndCalculateJSON(app, nodeIP)
		// }
		
		// check bundlelist is not None
		bundlelist := GetPulledBundleNamesFromFile(nodeIP)
		if bundlelist == nil || len(bundlelist) == 0 {
			sizes = .0
		}else{
			sizes = CompareAndCalculateJSON(app, nodeIP)
		}
	}

	handleReponse(w, r, sizes)
}

func main() {
	klog.InitFlags(nil)
	var err error

	// for idx := 1; idx <= 1000; idx++ {
	// 	folderName := fmt.Sprintf("/10.0.%d.%d", idx/250, idx%250+1)
	// 	// bm, err = bundle.NewBundleManager(workDir, upstramSvc)
	// 	bms[folderName], err = bundle.NewBundleManager(workDir+folderName, upstramSvc)
	// 	if err != nil {
	// 		klog.Fatalf("[Bundle Daemon] Failed to create BundleManager: %v", err)
	// 	}
	// }
	
	http.HandleFunc("/bundles/", bundleHandler)
	http.HandleFunc("/layers/", layerHandler)

	klog.Info(fmt.Sprintf("[Blob Daemon] Starting HTTP Server on :%s", endPort))
	err = http.ListenAndServe(fmt.Sprintf(":%s", endPort), nil)
	if err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}
