//go:build load

package load

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/test/internal/kubernetes"
	"github.com/stretchr/testify/require"
)

const (
	// AKS-managed Microsoft Defender (azsecpack) DaemonSet in kube-system. After
	// a VMSS reboot its init container can wedge in a saturated CrashLoopBackOff.
	// The pod selector is derived from the DaemonSet spec at runtime, so only the
	// name is needed here.
	defenderNamespace     = "kube-system"
	defenderDaemonsetName = "azuresecuritylinuxagent"
	// How long to let a not-Running Defender pod finish its post-reboot init
	// before force-restarting it.
	defenderSettleTimeout = 5 * time.Minute
)

// TestEnsureSystemPodsReady is a post-node-restart readiness gate meant to run
// before the upstream Kubernetes e2e suite. That suite's SynchronizedBeforeSuite
// requires every kube-system pod to be Running+Ready within a fixed budget; a
// Defender DaemonSet pod wedged in a CrashLoopBackOff after a VMSS reboot can
// miss that budget and fail the whole suite with "Ran 0 of N Specs". This waits
// for the cluster to settle and force-restarts any wedged Defender pod so it can
// recover before the suite runs.
func TestEnsureSystemPodsReady(t *testing.T) {
	clientset := kubernetes.MustGetClientset()
	// Keep the deadline below the make target's `go test -timeout 30m` so a
	// worst-case wait cancels gracefully via context rather than a hard timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	err := kubernetes.RecoverDaemonsetAfterRestart(ctx, clientset, defenderNamespace, defenderDaemonsetName, defenderSettleTimeout)
	require.NoError(t, err, "failed to ensure system pods are ready after node restart")
}
