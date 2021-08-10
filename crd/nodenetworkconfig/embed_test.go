package manifests

import (
	"os"
	"reflect"
	"testing"
)

const filename = "manifests/acn.azure.com_nodenetworkconfigs.yaml"

func TestEmbed(t *testing.T) {
	b, err := os.ReadFile(filename)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(NodeNetworkConfigsYAML, b) {
		t.Errorf("embedded file did not match file on disk")
	}
}
