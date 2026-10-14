package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadyChecker(t *testing.T) {
	t.Parallel()
	const notReadyBody = "internal server error: not ready\n"
	tests := []struct {
		name              string
		started           bool
		requireConflist   bool
		conflistGenerated bool
		wantStatus        int
		wantBody          string
	}{
		{
			name:            "startup incomplete",
			requireConflist: true,
			wantStatus:      http.StatusInternalServerError,
			wantBody:        notReadyBody,
		},
		{
			name:              "conflist alone does not complete startup",
			requireConflist:   true,
			conflistGenerated: true,
			wantStatus:        http.StatusInternalServerError,
			wantBody:          notReadyBody,
		},
		{
			name:       "startup incomplete without generation",
			wantStatus: http.StatusInternalServerError,
			wantBody:   notReadyBody,
		},
		{
			name:            "startup complete but conflist pending",
			started:         true,
			requireConflist: true,
			wantStatus:      http.StatusInternalServerError,
			wantBody:        "internal server error: cni conflist not ready\n",
		},
		{
			name:              "startup and conflist complete",
			started:           true,
			requireConflist:   true,
			conflistGenerated: true,
			wantStatus:        http.StatusOK,
			wantBody:          "ok",
		},
		{
			name:       "generation disabled",
			started:    true,
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			started := make(chan any)
			if tt.started {
				close(started)
			}
			var checkedConflist bool
			checker := newReadyChecker(started, tt.requireConflist, func() bool {
				checkedConflist = true
				return tt.conflistGenerated
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				"/readyz",
				http.NoBody,
			)
			checker.ServeHTTP(response, request)

			assert.Equal(t, tt.wantStatus, response.Code)
			assert.Equal(t, tt.wantBody, response.Body.String())
			assert.Equal(t, tt.started && tt.requireConflist, checkedConflist)
		})
	}
}
