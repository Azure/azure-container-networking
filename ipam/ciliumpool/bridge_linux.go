package ciliumpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/store"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

var (
	errBridgeConfig       = errors.New("ciliumpool: incomplete bridge configuration")
	errLocalState         = errors.New("ciliumpool: invalid local state")
	errUnsupportedRelease = errors.New("ciliumpool: cloud release is unsupported")
)

// Bridge serializes usage publication. Reconcile must not be called concurrently.
type Bridge struct {
	Config               *common.CiliumPoolConfig
	Client               dynamic.Interface
	Discover             func(context.Context, *common.CiliumPoolConfig) (Guest, error)
	Logger               *zap.Logger
	resourceVersion      string
	lastUsagePublication time.Time
}

func (b *Bridge) Run(ctx context.Context) error {
	if b.Config == nil || b.Client == nil || b.Discover == nil || b.Logger == nil {
		return errBridgeConfig
	}
	if err := os.MkdirAll(filepath.Join(b.Config.StateDir, "locks"), 0o700); err != nil {
		return fmt.Errorf("creating bridge lock directory: %w", err)
	}
	lifetime, err := acquire(ctx, b.Config.LockPath(consumer), false)
	if err != nil {
		return fmt.Errorf("acquiring bridge lifetime lock: %w", err)
	}
	defer closeLock(lifetime, b.Logger)
	var w watch.Interface
	defer func() {
		if w != nil {
			w.Stop()
		}
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err = b.Reconcile(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.Logger.Warn("pool synchronization failed", zap.Error(err))
		}
		if w == nil {
			seconds := int64(300)
			w, err = b.Client.Resource(CiliumNodes).Watch(ctx, metav1.ListOptions{
				FieldSelector:   "metadata.name=" + b.Config.NodeName,
				ResourceVersion: b.resourceVersion, TimeoutSeconds: &seconds,
			})
			if err != nil {
				b.Logger.Warn("pool watch failed", zap.Error(err))
				w = nil
			}
		}
		var events <-chan watch.Event
		if w != nil {
			events = w.ResultChan()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case event, ok := <-events:
			if !ok || event.Type == watch.Error {
				w.Stop()
				w = nil
				// Periodic GETs also recover expired resource versions and lost events.
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
				}
			}
		}
	}
}

func (b *Bridge) Reconcile(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	g, err := b.Discover(ctx, b.Config)
	if err != nil {
		return err
	}
	rawNode, err := b.Client.Resource(Nodes).Get(ctx, b.Config.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading node: %w", err)
	}
	var node corev1.Node
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(rawNode.Object, &node); err != nil {
		return fmt.Errorf("decoding node: %w", err)
	}
	cn, err := b.Client.Resource(CiliumNodes).Get(ctx, node.Name, metav1.GetOptions{})
	initialize := false
	if apierrors.IsNotFound(err) {
		for _, name := range []string{"azure-vnet", "azure-vnet-ipam"} {
			if _, statErr := os.Stat(b.Config.StatePath(name)); !os.IsNotExist(statErr) {
				return errors.Join(fmt.Errorf("%w: missing cilium node with existing local state", errLocalState),
					b.fence(ctx, "cilium node deleted"))
			}
		}
		cn, err = b.registration(&node, g)
		if err != nil {
			return err
		}
		cn, err = b.Client.Resource(CiliumNodes).Create(ctx, cn, metav1.CreateOptions{})
		initialize = err == nil
	}
	if err != nil {
		return fmt.Errorf("registering or reading cilium node: %w", err)
	}
	id, err := identity(b.Config, &node, cn, g)
	if err != nil {
		return errors.Join(err, b.fence(ctx, "node identity or consumer changed"))
	}
	// Persist the binding before the first operator pool publication. A crash
	// between API creation and this write intentionally requires fenced recovery.
	if initialize {
		if _, _, err = b.transaction(ctx, id, nil, true, false); err != nil {
			return err
		}
	}
	if release, found, releaseErr := unstructured.NestedMap(cn.Object, "status", "ipam", "release-ips"); releaseErr != nil {
		return fmt.Errorf("reading release handshake: %w", releaseErr)
	} else if found && len(release) != 0 {
		return errors.Join(errUnsupportedRelease,
			b.fence(ctx, "unexpected cloud release handshake"))
	}
	supply, total, err := eligible(cn, id, g)
	if err != nil {
		return err
	}
	observedAt := time.Now()
	used, quarantined, err := b.transaction(ctx, id, supply, false, false)
	if err != nil {
		return err
	}
	patch, err := usagePatch(cn, used)
	if err != nil {
		return err
	}
	var current nodeContract
	data, err := cn.MarshalJSON()
	if err != nil {
		return fmt.Errorf("encoding usage status: %w", err)
	}
	if err = json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decoding usage status: %w", err)
	}
	updated := cn
	if current.Status.IPAM.Used == nil || !maps.Equal(current.Status.IPAM.Used, used) {
		updated, err = b.Client.Resource(CiliumNodes).Patch(ctx, node.Name, types.JSONPatchType,
			patch, metav1.PatchOptions{}, "status")
		if err != nil {
			// The next attempt re-reads BOTH local usage and the API object.
			return fmt.Errorf("publishing usage: %w", err)
		}
		b.lastUsagePublication = time.Now()
	}
	b.resourceVersion = updated.GetResourceVersion()
	if updated.GetUID() != cn.GetUID() {
		return errors.Join(fmt.Errorf("%w: cilium node replaced during usage publication", errInvalidContract),
			b.fence(ctx, "cilium node UID changed"))
	}
	if _, _, err = b.transaction(ctx, id, supply, false, true); err != nil {
		return err
	}
	free := 0
	for _, entry := range supply {
		if _, reserved := used[entry.Address.String()]; !reserved {
			free++
		}
	}
	b.Logger.Info("pool synchronized",
		zap.Int("pool", total), zap.Int("eligible", len(supply)),
		zap.Int("used", len(used)), zap.Int("quarantined", quarantined),
		zap.Int("free", free),
		zap.Int("reserveDeficit", max(0, b.Config.PreAllocate-free)),
		zap.Time("lastPoolObservation", observedAt), zap.Time("usageConfirmedAt", time.Now()),
		zap.Time("lastUsagePublication", b.lastUsagePublication))
	if quarantined > 0 {
		b.Logger.Warn("assigned addresses withdrawn from eligible supply", zap.Int("quarantined", quarantined))
	}
	return nil
}

func (b *Bridge) registration(node *corev1.Node, g Guest) (*unstructured.Unstructured, error) {
	instance, err := resourceID(g.InstanceID, "Microsoft.Compute/virtualMachines")
	if err != nil {
		return nil, err
	}
	cn := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cilium.io/v2", "kind": "CiliumNode",
		"metadata": map[string]interface{}{
			"name": node.Name, "uid": "validation-only",
			"annotations": map[string]interface{}{consumerAnnotation: consumer},
			"ownerReferences": []interface{}{map[string]interface{}{
				"apiVersion": "v1", "kind": "Node", "name": node.Name, "uid": string(node.UID),
			}},
		},
		"spec": map[string]interface{}{
			"instance-id": instance,
			"addresses": []interface{}{map[string]interface{}{
				"type": "InternalIP", "ip": g.Primary.String(),
			}},
			"azure": map[string]interface{}{"interface-name": b.Config.NICID[strings.LastIndex(b.Config.NICID, "/")+1:]},
			"ipam": map[string]interface{}{
				"min-allocate": int64(b.Config.MinAllocate),
				"pre-allocate": int64(b.Config.PreAllocate),
			},
		},
	}}
	if _, err := identity(b.Config, node, cn, g); err != nil {
		return nil, err
	}
	cn.SetUID("")
	return cn, nil
}

func (b *Bridge) transaction(ctx context.Context, id ipam.CiliumPoolIdentity,
	supply []ipam.CiliumPoolAddress, initialize, admit bool,
) (used map[string]ipam.CiliumPoolUsage, quarantined int, err error) {
	// Reading network-state existence is recovery work: respect CNI's lock order.
	network, err := acquire(ctx, b.Config.LockPath("azure-vnet"), true)
	if err != nil {
		return nil, 0, err
	}
	defer closeLock(network, b.Logger)
	lock, err := acquire(ctx, b.Config.LockPath("azure-vnet-ipam"), true)
	if err != nil {
		return nil, 0, err
	}
	defer closeLock(lock, b.Logger)
	kvs, err := store.NewDurableJSONFileStore(b.Config.StatePath("azure-vnet-ipam"), nil, b.Logger)
	if err != nil {
		return nil, 0, fmt.Errorf("opening local IPAM state: %w", err)
	}
	used, quarantined, err = ipam.CiliumPoolTransaction(kvs, id, supply, initialize, admit)
	if err != nil {
		return nil, 0, fmt.Errorf("synchronizing local pool: %w", err)
	}
	if len(used) > 0 {
		networkData, err := os.ReadFile(b.Config.StatePath("azure-vnet"))
		var networkState map[string]json.RawMessage
		if err == nil {
			err = json.Unmarshal(networkData, &networkState)
			if err == nil && len(networkState["Network"]) == 0 {
				err = fmt.Errorf("%w: missing Network store key", errLocalState)
			}
		}
		if err != nil {
			return nil, 0, errors.Join(fmt.Errorf("ciliumpool: network state missing with active reservations: %w", err),
				ipam.FenceCiliumPool(kvs, "network state missing"))
		}
	}
	return used, quarantined, nil
}

func (b *Bridge) fence(ctx context.Context, reason string) error {
	lock, err := acquire(ctx, b.Config.LockPath("azure-vnet-ipam"), true)
	if err != nil {
		return err
	}
	defer closeLock(lock, b.Logger)
	kvs, err := store.NewDurableJSONFileStore(b.Config.StatePath("azure-vnet-ipam"), nil, b.Logger)
	if err != nil {
		return fmt.Errorf("opening local IPAM state for fencing: %w", err)
	}
	if err := ipam.FenceCiliumPool(kvs, reason); err != nil {
		return fmt.Errorf("fencing local pool: %w", err)
	}
	return nil
}

func acquire(ctx context.Context, path string, wait bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening state lock: %w", err)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for state lock: %w", errors.Join(err, f.Close()))
		}
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !wait || (!errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR)) {
			return nil, fmt.Errorf("acquiring state lock: %w", errors.Join(err, f.Close()))
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for state lock: %w", errors.Join(ctx.Err(), f.Close()))
		case <-ticker.C:
		}
	}
}

func closeLock(f *os.File, logger *zap.Logger) {
	if err := f.Close(); err != nil {
		logger.Warn("closing state lock failed", zap.Error(err))
	}
}
