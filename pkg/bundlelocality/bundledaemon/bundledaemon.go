package main

import (
	"k8s.io/klog/v2"
	"net"
	"net/http"
	"strconv"
)

const svcPort = "9998"

func queryLocalTaskC(nodeIP, bundleName, bundleVersion string) bool {
	klog.Infof("Query Local TaskC IP: %v, Name: %v, Ver: %v, exists: true", nodeIP, bundleName, bundleVersion)
	return true
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

	http.HandleFunc("/bundle", bundleHandler)

	klog.Info(fmt.Sprintf("[Bundle Daemon] Starting HTTP Server on :%s", svcPort))
	err := http.ListenAndServe(fmt.Sprintf(":%s", svcPort), nil)
	if err != nil {
		klog.Fatalf("Failed to start server: %v", err)
	}
}
