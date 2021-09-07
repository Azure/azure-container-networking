package nodenetworkconfig

import (
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

func TestSpecToJSON(t *testing.T) {
	tests := []struct {
		name    string
		spec    *v1alpha.NodeNetworkConfigSpec
		want    []byte
		wantErr bool
	}{
		{
			name: "good",
			spec: &v1alpha.NodeNetworkConfigSpec{
				RequestedIPCount: 13,
				IPsNotInUse:      []string{"abc", "def"},
			},
			want:    []byte(`{"spec":{"requestedIPCount":13,"ipsNotInUse":["abc","def"]}}`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := specToJSON(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("specToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("specToJSON() = %s, want %s", got, tt.want)
			}
		})
	}
}
