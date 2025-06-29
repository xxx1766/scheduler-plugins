package bundlelocality

import (
	"testing"
)

func TestQueryNodeBundles(t *testing.T) {
	remoteBundles := GetContainerBundles(normalizedBundleName("yolo11"))

	/* for _, b := range remoteBundles {
		t.Logf("Remote Bundle: %s, Type: %s, Version: %s, Size: %.2f MiB", b.name, b.specType, b.specifier, b.size)
	} */

	QueryNodeBundles("127.0.0.1", remoteBundles)
}
