// adapted from `scheduler-plugins/pkg/podstate/pod_state.go` and
// kubernetes/pkg/scheduler/framework/plugins/imagelocality/image_locality.go`

package layerlocality

import (
	"bytes"
	"context"
	"fmt"
	"math"

	// "math"
	"strings"

	"encoding/json"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"k8s.io/klog/v2"
)

// The two thresholds are used as bounds for the layer score range. They correspond to a reasonable size range for
// prefab layers compressed and stored in registries; 90%ile of layers on dockerhub drops into this range.
const (
	mb                    int64  = 1024 * 1024
	minThreshold          int64  = 20 * mb  // 24117248
	maxContainerThreshold int64  = 100 * mb // 1048576000
	endPort               string = "9998"
	upstramSvc            string = "https://prefab.cs.ac.cn:10062"
)

// refer to `TaskC/pkg/prefab/prefab.go` `type Prefab struct`
type RemotePrefabInfo struct {
	SpecType  string  `json:"spectype"`  // e.g., "image", "package", etc.
	Name      string  `json:"name"`      // for layers, this is layer digest
	Specifier string  `json:"specifier"` // e.g., "v1.0.0", "latest", etc.
	Size      float64 `json:"size"`      // in MiB
}

// LayerLocality is a score plugin that favors nodes that already have requested pod container's layers.
type LayerLocality struct {
	logger klog.Logger
	handle framework.Handle
}

var _ framework.ScorePlugin = &LayerLocality{}

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "LayerLocality"

// Name returns name of the plugin. It is used in logs, etc.
func (ll *LayerLocality) Name() string {
	return Name
}

// Score invoked at the score extension point.
func (ll *LayerLocality) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// logger := bl.logger
	// klog.InfoS("[Locality] Scoring Pods Start...")
	//logger.Info("{Locality} Scoring Pods Start...")
	nodeInfos, err := ll.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	totalNumNodes := len(nodeInfos)

	nodeInfo, err := ll.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	layerScores := sumLayerScores(nodeInfo, pod, totalNumNodes)
	score := calculatePriority(layerScores, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	klog.InfoS(fmt.Sprintf("[Layer Locality] Scoring Pods End (score = %d)...", score))
	//logger.Info(fmt.Sprintf("{Layer Locality} Scoring Pods End (score = %d)...", score))
	return score, nil
}

// ScoreExtensions of the Score plugin.
func (ll *LayerLocality) ScoreExtensions() framework.ScoreExtensions {
	return nil // ll
}

/* func (ll *LayerLocality) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	// klog.InfoS("[Layer Locality] Normalize Score Start...")
	var highest int64 = -math.MaxInt64
	var lowest int64 = math.MaxInt64
	for _, nodeScore := range scores {
		if nodeScore.Score > highest {
			highest = nodeScore.Score
		}
		if nodeScore.Score < lowest {
			lowest = nodeScore.Score
		}
	}

	oldRange := highest - lowest
	newRange := framework.MaxNodeScore - framework.MinNodeScore
	for i, nodeScore := range scores {
		if oldRange == 0 {
			scores[i].Score = framework.MinNodeScore
		} else {
			scores[i].Score = ((nodeScore.Score - lowest) * newRange / oldRange) + framework.MinNodeScore
		}
	}

	klog.InfoS("[Locality] Normalize Score End...")
	return nil
} */

// New initializes a new plugin and returns it.
func New(ctx context.Context, _ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	klog.Background().Info("[Layer Locality] Registering...")
	initUpstreamClient()
	logger := klog.FromContext(ctx).WithValues("plugin", Name)
	return &LayerLocality{logger: logger, handle: h}, nil
}

// calculatePriority returns the priority of a node. Given the sumScores of requested layers on the node, the node's
// priority is obtained by scaling the maximum priority value with a ratio proportional to the sumScores.
func calculatePriority(sumScores int64, numContainers int) int64 {
	maxThreshold := maxContainerThreshold * int64(numContainers)
	if sumScores < minThreshold {
		sumScores = minThreshold
	} else if sumScores > maxThreshold {
		sumScores = maxThreshold
	}

	return framework.MaxNodeScore * (sumScores - minThreshold) / (maxThreshold - minThreshold)
}

func QueryNodeLayersWrapper(nodeInfo *framework.NodeInfo, layers []RemotePrefabInfo) float64 {
	var nodeAddresses []v1.NodeAddress = nodeInfo.Node().Status.Addresses

	// Samples:
	/* []v1.NodeAddress{
		{Type: v1.NodeInternalIP, Address: "127.0.0.1"},
		{Type: v1.NodeInternalIP, Address: "127.0.0.2"},
		{Type: v1.NodeInternalIP, Address: "127.0.0.3"},
		{Type: v1.NodeHostName, Address: "MyHostName"},
	} */

	for _, address := range nodeAddresses {
		if address.Type == v1.NodeInternalIP {
			nodeAddress := address.Address
			klog.Infof("[Layer Locality] Querying node %s for layers...", nodeAddress)
			return QueryNodeLayers(nodeAddress, layers)
		}
	}

	// should not happen in test cases, but just in case
	for _, address := range nodeAddresses {
		if address.Type == v1.NodeExternalIP {
			nodeAddress := address.Address
			// klog.Infof("[Layer Locality] Querying node %s for layers...", nodeAddress)
			return QueryNodeLayers(nodeAddress, layers)
		}
	}

	// If no suitable node address is found, log a warning and return 0
	klog.Warning("[Layer Locality] No suitable node address found for querying layers.")
	return .0 // Return 0 if no suitable node address is found
}

func QueryNodeLayers(nodeAddress string, layers []RemotePrefabInfo) float64 {
	/* for real test */
	// klog.Infof("[Layer Locality] Trying to query http://%s:%s/layers", nodeAddress, endPort)
	// baseURL := fmt.Sprintf("http://%s:%s/layers", nodeAddress, endPort)
	
	/* for simulating test */
	baseURL := fmt.Sprintf("http://localhost:%s/layers/%s", endPort, nodeAddress)
	klog.Infof("[Layer Locality] Trying to query %s", baseURL)

	/* params := url.Values{}
	params.Add("name", layerName)
	params.Add("version", layerVersion)
	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode()) */

	sizes := .0

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	/* for _, b := range layers {
		klog.Infof("[Layer Locality] [Before JSON Marshal] Remote Layer: %s, Type: %s, Version: %s, Size: %.2f MiB", b.Name, b.SpecType, b.Specifier, b.Size)
	} */

	payload, err := json.Marshal(layers)
	if err != nil {
		klog.Errorf("[Layer Locality] failed to marshal layer info: %v", err)
		return sizes
	}

	req, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(payload))
	if err != nil {
		klog.Errorf("[Layer Locality] failed to create request: %v", err)
		return sizes
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		klog.Warningf("[Layer Locality] Error querying node %s: %v\n", nodeAddress, err)
		return sizes
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		klog.Warningf("[Layer Locality] Non-OK HTTP status: %v from node %s\n", resp.StatusCode, nodeAddress)
		return sizes
	}

	var response struct {
		Sizes float64 `json:"sizes"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		klog.Warningf("[Layer Locality] Failed to decode response from node %s: %v\n", nodeAddress, err)
		return sizes
	}

	klog.Infof("[Layer Locality] Received sizes: %.2f Bytes from node %s", response.Sizes, nodeAddress)

	return response.Sizes
}

// sumLayerScores returns the sum of layer scores of all the containers that are already on the node.
// Each layer receives a raw score of its size, scaled by scaledImageScore. The raw scores are later used to calculate
// the final score.
func sumLayerScores(nodeInfo *framework.NodeInfo, pod *v1.Pod, totalNumNodes int) int64 {
	var sum int64 = 0

	allContainers := append(pod.Spec.InitContainers, pod.Spec.Containers...)

	/* for _, container := range pod.Spec.InitContainers {
		if state, ok := nodeInfo.ImageStates[normalizedLayerName(container.Image)]; ok {
			sum += scaledImageScore(state, totalNumNodes)
		}
	}
	for _, container := range pod.Spec.Containers {
		if state, ok := nodeInfo.ImageStates[normalizedLayerName(container.Image)]; ok {
			sum += scaledImageScore(state, totalNumNodes)
		}
	} */

	for _, container := range allContainers {
		/* if state, ok := nodeInfo.ImageStates[normalizedLayerName(container.Image)]; ok {
			sum += scaledImageScore(state, totalNumNodes)
			klog.Infof("[Layer Locality] [ImgCmp] sum += %v\n", sum)
		} */ // currently, image size is broken, to be fixed by other developers

		sizes := QueryNodeLayersWrapper(nodeInfo, GetContainerLayers(normalizedImageName(container.Image)))
		// klog.Infof("[Layer Locality] [PakCmp Before] sizes=%v, totalNumNodes=%v, sum+=%v\n", sizes, float64(totalNumNodes), sum)

		scalingFactor := math.Sqrt(float64(len(nodeInfo.Pods) + 1))

		sum += int64(float64(sizes) / float64(scalingFactor))

		klog.Infof("[Layer Locality] [rawScore] size(before scaling)=%v, len(nodeInfo.Pods)=%v, totalNumNodes=%v\n", sizes, len(nodeInfo.Pods), float64(totalNumNodes))
	}

	return sum
}

// scaledImageScore returns an adaptively scaled score for the given state of an image.
// The size of the image is used as the base score, scaled by a factor which considers how much nodes the image has "spread" to.
// This heuristic aims to mitigate the undesirable "node heating problem", i.e., pods get assigned to the same or
// a few nodes due to image locality.
func scaledImageScore(imageState *framework.ImageStateSummary, totalNumNodes int) int64 {
	spread := float64(imageState.NumNodes) / float64(totalNumNodes)
	// klog.Infof("[Layer Locality] [scaledImageScore] float64(imageState.NumNodes)=%v, float64(totalNumNodes)=%v, float64(imageState.Size)=%v\n", float64(imageState.NumNodes), float64(totalNumNodes), float64(imageState.Size))
	return int64(10.0 /*float64(imageState.Size)*/ * spread) // need to be fixed by other developers
}

// normalizedImageName returns the CRI compliant name for a given layer.
// TODO: cover the corner cases of missed matches, e.g,
// 1. Using Docker as runtime and docker.io/library/test:tag in pod spec, but only test:tag will present in node status
// 2. Using the implicit registry, i.e., test:tag or library/test:tag in pod spec but only docker.io/library/test:tag
// in node status; note that if users consistently use one registry format, this should not happen.
func normalizedImageName(name string) string {
	if strings.LastIndex(name, ":") <= strings.LastIndex(name, "/") {
		name = name + ":latest"
	}
	return name
}
