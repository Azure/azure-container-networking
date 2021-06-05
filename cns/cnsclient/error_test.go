package cnsclient

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns/types"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "is not found",
			err: &CNSClientError{
				Code: types.UnknownContainerID,
				Err:  fmt.Errorf("unknown container id"),
			},
			want: true,
		},
		{
			name: "is not cnsclienterr",
			err:  fmt.Errorf("error"),
			want: false,
		},
		{
			name: "is other cnsclienterr",
			err: &CNSClientError{
				Code: types.UnexpectedError,
				Err:  fmt.Errorf("unexpected err"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
