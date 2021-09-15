package crd

import (
	"errors"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsNotDefined tells whether the given error is a CRD not defined error
func IsNotDefined(err error) bool {
	if !apierr.IsNotFound(err) {
		return false
	}
	statusErr := &apierr.StatusError{}
	if !errors.As(err, &statusErr) {
		return false
	}
	for _, cause := range statusErr.ErrStatus.Details.Causes {
		if cause.Type == metav1.CauseTypeUnexpectedServerResponse {
			return true
		}
	}
	return false
}
