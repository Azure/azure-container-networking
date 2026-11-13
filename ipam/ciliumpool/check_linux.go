package ciliumpool

import (
	"context"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *Allocator) Check(ctx context.Context, owner Owner) error {
	node, err := a.Nodes.Get(ctx, a.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading CiliumNode: %w", err)
	}
	if callErr := validateCiliumNode(node, a.Binding); callErr != nil {
		return callErr
	}
	state, _, err := readState(node, a.Binding)
	if err != nil {
		return err
	}
	supply, err := eligibleSupply(node, a.Binding, a.Supply)
	if err != nil {
		return err
	}
	for ip, entry := range state.Reservations {
		if entry.Owner == owner && entry.BootID == a.BootID && slices.Contains(supply, ip) {
			return nil
		}
	}
	return fmt.Errorf("%w: no current eligible reservation", ErrOwnership)
}
