package ciliumpool

import (
	"strings"
	"testing"
)

func TestRemovedReleaseSettingsAreRejectedEvenWhenFalse(t *testing.T) {
	for _, setting := range []string{"releaseExcessIPs", "coordinatePeers"} {
		raw := strings.Replace(testConfig, `"preAllocate":8`, `"preAllocate":8,"`+setting+`":false`, 1)
		if _, _, err := ParseConfig([]byte(raw)); err == nil {
			t.Fatalf("removed setting %s was silently accepted", setting)
		}
	}
}
