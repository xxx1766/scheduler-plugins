// adapted from `scheduler-plugins/pkg/podstate/pod_state.go` and
// kubernetes/pkg/scheduler/framework/plugins/imagelocality/image_locality.go`

package bundlelocality

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"k8s.io/klog/v2"
)

// The two thresholds are used as bounds for the bundle score range. They correspond to a reasonable size range for
// prefab bundles compressed and stored in registries; 90%ile of bundles on dockerhub drops into this range.
const (
	mb                    int64 = 1024 * 1024
	minThreshold          int64 = 23 * mb
	maxContainerThreshold int64 = 1000 * mb
)

// BundleLocality is a score plugin that favors nodes that already have requested pod container's bundles.
type BundleLocality struct {
	logger klog.Logger
	handle framework.Handle
}

var _ framework.ScorePlugin = &BundleLocality{}

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "BundleLocality"

// Name returns name of the plugin. It is used in logs, etc.
func (bl *BundleLocality) Name() string {
	return Name
}

// Score invoked at the score extension point.
func (bl *BundleLocality) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// logger := bl.logger
	klog.Background().Info("[Bundle Locality] Scoring Pods Start...")
	//logger.Info("{Bundle Locality} Scoring Pods Start...")
	nodeInfos, err := bl.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	totalNumNodes := len(nodeInfos)

	nodeInfo, err := bl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	bundleScores := sumBundleScores(nodeInfo, pod, totalNumNodes)
	score := calculatePriority(bundleScores, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	klog.Background().Info(fmt.Sprintf("{Bundle Locality} Scoring Pods End (score = %d)...", score))
	//logger.Info(fmt.Sprintf("{Bundle Locality} Scoring Pods End (score = %d)...", score))
	return score, nil
}

// ScoreExtensions of the Score plugin.
func (bl *BundleLocality) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// New initializes a new plugin and returns it.
func New(ctx context.Context, _ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	klog.Background().Info("[Bundle Locality] Registering...")
	logger := klog.FromContext(ctx).WithValues("plugin", Name)
	return &BundleLocality{logger: logger, handle: h}, nil
}

// calculatePriority returns the priority of a node. Given the sumScores of requested bundles on the node, the node's
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

// sumBundleScores returns the sum of bundle scores of all the containers that are already on the node.
// Each bundle receives a raw score of its size, scaled by scaledBundleScore. The raw scores are later used to calculate
// the final score.
func sumBundleScores(nodeInfo *framework.NodeInfo, pod *v1.Pod, totalNumNodes int) int64 {
	var sum int64
	for _, container := range pod.Spec.InitContainers {
		if state, ok := nodeInfo.ImageStates[normalizedBundleName(container.Image)]; ok {
			sum += scaledBundleScore(state, totalNumNodes)
		}
	}
	for _, container := range pod.Spec.Containers {
		if state, ok := nodeInfo.ImageStates[normalizedBundleName(container.Image)]; ok {
			sum += scaledBundleScore(state, totalNumNodes)
		}
	}
	return sum
}

// scaledBundleScore returns an adaptively scaled score for the given state of a bundle.
// The size of the bundle is used as the base score, scaled by a factor which considers how much nodes the bundle has "spread" to.
// This heuristic aims to mitigate the undesirable "node heating problem", i.e., pods get assigned to the same or
// a few nodes due to bundle locality.
func scaledBundleScore(bundleState *framework.ImageStateSummary, totalNumNodes int) int64 {
	spread := float64(bundleState.NumNodes) / float64(totalNumNodes)
	return int64(float64(bundleState.Size) * spread)
}

// normalizedBundleName returns the CRI compliant name for a given bundle.
// TODO: cover the corner cases of missed matches, e.g,
// 1. Using Docker as runtime and docker.io/library/test:tag in pod spec, but only test:tag will present in node status
// 2. Using the implicit registry, i.e., test:tag or library/test:tag in pod spec but only docker.io/library/test:tag
// in node status; note that if users consistently use one registry format, this should not happen.
func normalizedBundleName(name string) string {
	if strings.LastIndex(name, ":") <= strings.LastIndex(name, "/") {
		name = name + ":latest"
	}
	return name
}
