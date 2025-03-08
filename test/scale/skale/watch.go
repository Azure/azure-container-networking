package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
)

var watchcmd = &cobra.Command{
	Use:   "watch",
	Short: "Collect metrics for NNC and Node events",
	RunE:  watchE,
}

func init() {
	rootcmd.AddCommand(watchcmd)
}

func watchE(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	z.Debug("opening watches")
	nncch := make(chan *v1alpha.NodeNetworkConfig)
	nncw, err := dynacli.Resource(schema.GroupVersionResource{
		Group:    v1alpha.GroupVersion.Group,
		Version:  v1alpha.GroupVersion.Version,
		Resource: "nodenetworkconfigs",
	}).Namespace("kube-system").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	nodech := make(chan *corev1.Node)
	nodew, err := kubecli.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{
		LabelSelector: "type=kwok",
	})
	if err != nil {
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go process(ctx, nncch, nodech, wg.Done)
	go pipe(nncw, nncch, convNNC, wg.Done)
	go pipe(nodew, nodech, convNode, wg.Done)
	wg.Wait()
	return nil
}

func convNNC(obj runtime.Object) *v1alpha.NodeNetworkConfig {
	u := obj.(*unstructured.Unstructured)
	bytes, _ := u.MarshalJSON()
	var nnc v1alpha.NodeNetworkConfig
	json.Unmarshal(bytes, &nnc)
	return &nnc
}

func convNode(obj runtime.Object) *corev1.Node {
	return obj.(*corev1.Node)
}

func pipe[T runtime.Object](src watch.Interface, sink chan<- T, conv func(runtime.Object) T, done func()) {
	defer done()
	for {
		e, open := <-src.ResultChan()
		z.Debug("watch event", zap.String("object", e.Object.GetObjectKind().GroupVersionKind().String()))
		if !open {
			z.Debug("watch closed")
			break
		}
		sink <- conv(e.Object)
	}
}

func process(ctx context.Context, nncch <-chan *v1alpha.NodeNetworkConfig, nodech <-chan *corev1.Node, done func()) {
	defer done()
	events := map[string]event{}
	for {
		select {
		case nnc := <-nncch:
			// ignore non kwok nnc
			if !strings.Contains(nnc.Name, "skale") {
				continue
			}
			e := events[nnc.Name]
			e.nncCreation = nnc.GetCreationTimestamp().Time
			for _, f := range nnc.GetManagedFields() {
				if f.Manager == "dnc-rc" && f.Operation == "Update" && f.Subresource == "status" {
					e.nncReady = f.Time.Time
				}
			}
			events[nnc.Name] = e
		case node := <-nodech:
			e := events[node.Name]
			e.nodeCreation = node.GetCreationTimestamp().Time
			events[node.Name] = e
		case <-ctx.Done():
			return
		}
		pretty(events)
	}
}

func pretty(events map[string]event) {
	totals := struct {
		created               int64
		ready                 int64
		nncCreateLatencyAvgMs int64
		nncReadyLatencyAvgMs  int64
	}{}
	for i := range events {
		if events[i].created() {
			totals.created++
		}
		if events[i].nncCreateLatencyMs() > 0 {
			totals.nncCreateLatencyAvgMs = totals.nncCreateLatencyAvgMs*(totals.created-1)/totals.created + events[i].nncCreateLatencyMs()/totals.created
		}
		if events[i].ready() {
			totals.ready++
		}
		if events[i].nncReadyLatencyMs() > 0 {
			totals.nncReadyLatencyAvgMs = totals.nncReadyLatencyAvgMs*(totals.ready-1)/totals.ready + events[i].nncReadyLatencyMs()/totals.ready
		}
	}
	z.Debug("recalculated", zap.Int("total", len(events)), zap.Int64("create latency avg ms", totals.nncCreateLatencyAvgMs), zap.Int64("ready latency avg ms", totals.nncReadyLatencyAvgMs))
}

type event struct {
	nodeCreation time.Time
	nncCreation  time.Time
	nncReady     time.Time
}

func (e event) nncCreateLatencyMs() int64 {
	if e.nodeCreation.IsZero() || e.nncCreation.IsZero() {
		return -1
	}
	return e.nncCreation.Sub(e.nodeCreation).Milliseconds()
}

func (e event) nncReadyLatencyMs() int64 {
	if e.nncCreation.IsZero() || e.nncReady.IsZero() {
		return -1
	}
	return e.nncReady.Sub(e.nncCreation).Milliseconds()
}

func (e event) created() bool {
	return !e.nncCreation.IsZero()
}

func (e event) ready() bool {
	return !e.nncReady.IsZero()
}
