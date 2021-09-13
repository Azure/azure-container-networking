package cnsclient

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/pkg/errors"
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
				Err:  errors.New("unknown container id"),
			},
			want: true,
		},
		{
			name: "is not cnsclienterr",
			err:  errors.New("error"),
			want: false,
		},
		{
			name: "is other cnsclienterr",
			err: &CNSClientError{
				Code: types.UnexpectedError,
				Err:  errors.New("unexpected err"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "is not found",
			err: &CNSClientError{
				Code: types.UnknownContainerID,
				Err:  errors.New("unknown container id"),
			},
			want: fmt.Sprintf("[Azure CNSClient] Code: %d , Error: %v", types.UnknownContainerID, errors.New("unknown container id")),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
