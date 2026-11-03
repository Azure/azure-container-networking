package ciliumpool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"go.uber.org/zap"
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
	errUnsupportedRelease = errors.New("ciliumpool: cloud release is unsupported")
)

type poolState interface {
	Acquire(context.Context) (func(), error)
	CanInitialize(context.Context) error
	Synchronize(context.Context, ipam.CiliumPoolIdentity, []ipam.CiliumPoolAddress, bool, bool) (map[string]ipam.CiliumPoolUsage, int, error)
	Fence(context.Context, string) error
}

// Bridge serializes usage publication. Reconcile must not be called concurrently.
type Bridge struct {
	Config               *common.CiliumPoolConfig
	Client               dynamic.Interface
	Discover             func(context.Context, *common.CiliumPoolConfig) (Guest, error)
	Logger               *zap.Logger
	State                poolState
	Monitor              *Monitor
	resourceVersion      string
	lastUsagePublication time.Time
}

func (b *Bridge) Run(ctx context.Context) error {
	if b.Config == nil || b.Client == nil || b.Discover == nil || b.Logger == nil || b.State == nil {
		return errBridgeConfig
	}
	release, err := b.State.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring bridge consumer: %w", err)
	}
	defer release()
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

func (b *Bridge) Reconcile(ctx context.Context) (resultErr error) {
	started := time.Now()
	defer func() { b.Monitor.Observe("reconcile", time.Since(started), resultErr) }()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stageStarted := time.Now()
	g, err := b.Discover(ctx, b.Config)
	b.Monitor.Observe("guest", time.Since(stageStarted), err)
	if err != nil {
		return err
	}
	stageStarted = time.Now()
	rawNode, err := b.Client.Resource(Nodes).Get(ctx, b.Config.NodeName, metav1.GetOptions{})
	b.Monitor.Observe("node_api", time.Since(stageStarted), err)
	if err != nil {
		return fmt.Errorf("reading node: %w", err)
	}
	var node corev1.Node
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(rawNode.Object, &node); err != nil {
		return fmt.Errorf("decoding node: %w", err)
	}
	stageStarted = time.Now()
	cn, err := b.Client.Resource(CiliumNodes).Get(ctx, node.Name, metav1.GetOptions{})
	initialize := false
	if apierrors.IsNotFound(err) {
		if stateErr := b.State.CanInitialize(ctx); stateErr != nil {
			return errors.Join(stateErr, b.State.Fence(ctx, "cilium node deleted"))
		}
		cn, err = b.registration(&node, g)
		if err != nil {
			return err
		}
		cn, err = b.Client.Resource(CiliumNodes).Create(ctx, cn, metav1.CreateOptions{})
		initialize = err == nil
	}
	b.Monitor.Observe("pool_api", time.Since(stageStarted), err)
	if err != nil {
		return fmt.Errorf("registering or reading cilium node: %w", err)
	}
	id, err := identity(b.Config, &node, cn, g)
	if err != nil {
		return errors.Join(err, b.State.Fence(ctx, "node identity or consumer changed"))
	}
	// Persist the binding before the first operator pool publication. A crash
	// between API creation and this write intentionally requires fenced recovery.
	if initialize {
		if _, _, err = b.State.Synchronize(ctx, id, nil, true, false); err != nil {
			return fmt.Errorf("initializing pool binding: %w", err)
		}
	}
	if release, found, releaseErr := unstructured.NestedMap(cn.Object, "status", "ipam", "release-ips"); releaseErr != nil {
		return fmt.Errorf("reading release handshake: %w", releaseErr)
	} else if found && len(release) != 0 {
		return errors.Join(errUnsupportedRelease,
			b.State.Fence(ctx, "unexpected cloud release handshake"))
	}
	stageStarted = time.Now()
	supply, total, err := eligible(cn, id, g)
	b.Monitor.Observe("qualified_pool", time.Since(stageStarted), err)
	if err != nil {
		return err
	}
	b.Monitor.setPool(total)
	observedAt := time.Now()
	used, quarantined, err := b.State.Synchronize(ctx, id, supply, false, false)
	if err != nil {
		return fmt.Errorf("synchronizing pool ownership: %w", err)
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
		stageStarted = time.Now()
		updated, err = b.Client.Resource(CiliumNodes).Patch(ctx, node.Name, types.JSONPatchType,
			patch, metav1.PatchOptions{}, "status")
		b.Monitor.Observe("publication", time.Since(stageStarted), err)
		if err != nil {
			// The next attempt re-reads BOTH local usage and the API object.
			return fmt.Errorf("publishing usage: %w", err)
		}
		b.lastUsagePublication = time.Now()
	}
	b.resourceVersion = updated.GetResourceVersion()
	if updated.GetUID() != cn.GetUID() {
		return errors.Join(fmt.Errorf("%w: cilium node replaced during usage publication", errInvalidContract),
			b.State.Fence(ctx, "cilium node UID changed"))
	}
	b.Monitor.Observe("usage", time.Since(observedAt), nil)
	if _, _, err = b.State.Synchronize(ctx, id, supply, false, true); err != nil {
		return fmt.Errorf("enabling pool admissions: %w", err)
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
