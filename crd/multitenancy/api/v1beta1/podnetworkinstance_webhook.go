//go:build !ignore_uncovered
// +build !ignore_uncovered

package v1beta1

import (
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *PodNetworkInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
	if err != nil {
		return errors.Wrap(err, "failed to setup webhook")
	}
	return nil
}
