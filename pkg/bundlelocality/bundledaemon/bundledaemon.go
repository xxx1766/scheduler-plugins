package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"k8s.io/klog/v2"

	"github.com/L-F-Z/TaskC/pkg/bundle"
)

const (
	endPort    string = "9998"
	upstramSvc string = "https://prefab.cs.ac.cn:10062"
)

var bm *bundle.BundleManager

func queryLocalTaskC(nodeIP, bundleName, bundleVersion string) bool {
	klog.Infof("Query Local TaskC IP: %v, Name: %v, Ver: %v", nodeIP, bundleName, bundleVersion)
	bundles := bm.ListNames()
	if len(bundles) == 0 {
		// klog.Warningf("[Bundle Daemon] nodeIP=%v, No Task Bundle Exists.", nodeIP)
		return false
	}

	for _, b := range bundles { // "version" needs to be fixed...
		if b == bundleName+" ("+bundleVersion+")" {
			// klog.Infof("[Bundle Daemon] nodeIP=%v Found Bundle: %s", b, nodeIP)
			return true
		}
	}

	return false
}

func bundleHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	bundleName := query.Get("name")
	bundleVersion := query.Get("version")

	if bundleName == "" || bundleVersion == "" {
		http.Error(w, "missing 'name' or 'version' query param", http.StatusBadRequest)
		return
	}

	nodeIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		nodeIP = host
	}

	exists := queryLocalTaskC(nodeIP, bundleName, bundleVersion)

	w.Header().Set("Content-Type", "application/json")
	result := `{"bundle":"` + bundleName + `","version":"` + bundleVersion + `","exists":"` + strconv.FormatBool(exists) + `","size":1000}`
	w.Write([]byte(result))
}

func main() {
	klog.InitFlags(nil)
	var err error

	workDir := "/var/lib/taskc"
	bm, err = bundle.NewBundleManager(workDir, upstramSvc)
	if err != nil {
		klog.Fatalf("[Bundle Daemon] Failed to create BundleManager: %v", err)
	}

	http.HandleFunc("/bundle", bundleHandler)

	klog.Info(fmt.Sprintf("[Bundle Daemon] Starting HTTP Server on :%s", endPort))
	err = http.ListenAndServe(fmt.Sprintf(":%s", endPort), nil)
	if err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}
