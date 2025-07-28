package bundlelocality

import (
	"testing"
)

func TestCalPriority(t *testing.T) {
	t.Logf("Prio: %v\n", calculatePriority(12*mb, 1))
}

func TestQueryNodeBundles(t *testing.T) {
	remoteBundles := GetContainerBundles(normalizedBundleName("sam2:latest"))

	// t.Logf("remoteBundles[0]: %v, %v, %v\n", remoteBundles[0].Name, remoteBundles[0].Specifier, remoteBundles[0].Size)

	/* for _, b := range remoteBundles {
		t.Logf("Remote Bundle: %s, Type: %s, Version: %s, Size: %.2f MiB", b.Name, b.SpecType, b.Specifier, b.Size)
	} */

	QueryNodeBundles("127.0.0.1", remoteBundles)
}
