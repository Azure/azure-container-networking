package restserver

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/assert"
)

func TestResponseCodeToError(t *testing.T) {
	tests := []struct {
		name         string
		responseCode types.ResponseCode
		wantErr      bool
	}{
		{
			name:         "ok to nil",
			responseCode: types.Success,
			wantErr:      false,
		},
		{
			name:         "anything but ok to error",
			responseCode: types.UnknownContainerID,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ResponseCodeToError(tt.responseCode)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
