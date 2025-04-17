package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		if !open {
			z.Debug("watch closed")
			break
		}
		z.Debug("watch event", zap.String("object", e.Object.GetObjectKind().GroupVersionKind().String()))
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

type stats struct {
	total, avg, min, max, p50, p99 time.Duration
}

func (s *stats) MarshalLogObject(o zapcore.ObjectEncoder) error {
	o.AddInt64("total", int64(s.total))
	o.AddDuration("avg", s.avg)
	o.AddDuration("min", s.min)
	o.AddDuration("max", s.max)
	o.AddDuration("p50", s.p50)
	o.AddDuration("p99", s.p99)
	return nil
}

type snapshot struct {
	node, created, ready int
	t                    time.Time
}

type totals struct {
	nodes          int
	nncCreateStats stats
	nncReadyStats  stats
}

var timeline = map[int]snapshot{}

func pretty(events map[string]event) {
	totals := totals{}
	var createVals, readyVals []time.Duration
	for i := range events {
		if events[i].created() {
			totals.nncCreateStats.total++
		}
		if events[i].ready() {
			totals.nncReadyStats.total++
		}
		if val := events[i].nncCreateLatency(); val > 0 {
			totals.nncCreateStats.avg = totals.nncCreateStats.avg*(totals.nncCreateStats.total-1)/totals.nncCreateStats.total + val/totals.nncCreateStats.total
			createVals = append(createVals, val)
		}
		if val := events[i].nncReadyLatency(); val > 0 {
			totals.nncReadyStats.avg = totals.nncReadyStats.avg*(totals.nncReadyStats.total-1)/totals.nncReadyStats.total + val/totals.nncReadyStats.total
			readyVals = append(readyVals, val)
		}
	}
	if len(createVals) == 0 || len(readyVals) == 0 {
		z.Debug("no values")
		return
	}
	slices.Sort(createVals)
	slices.Sort(readyVals)
	totals.nodes = len(events)
	totals.nncCreateStats.max = createVals[len(createVals)-1]
	totals.nncCreateStats.min = createVals[0]
	totals.nncCreateStats.p50 = createVals[len(createVals)/2]
	totals.nncCreateStats.p99 = createVals[len(createVals)*99/100]
	totals.nncReadyStats.max = readyVals[len(readyVals)-1]
	totals.nncReadyStats.min = readyVals[0]
	totals.nncReadyStats.p50 = readyVals[len(readyVals)/2]
	totals.nncReadyStats.p99 = readyVals[len(readyVals)*99/100]
	z.Info("new recalculated", zap.Int("total", len(events)), zap.Object("create", &totals.nncCreateStats), zap.Object("ready", &totals.nncReadyStats))
	if totals.nncReadyStats.total == 100 {
		record(totals)
	}
}

func record2(events map[string]event) {
	// print node creation, nnc creation, nnc ready timestamps as csv columns
	for k, v := range events {
		fmt.Printf("%s,%d,%d,%d\n", k, v.nodeCreation.Unix(), v.nncCreation.Unix(), v.nncReady.Unix())
	}
	os.Exit(0)
}

func record(totals totals) {
	timeline[int(totals.nncReadyStats.total)] = snapshot{
		t:       time.Now(),
		node:    totals.nodes,
		created: int(totals.nncCreateStats.total),
		ready:   int(totals.nncReadyStats.total),
	}
	if totals.nncReadyStats.total%5000 == 0 {
		for _, s := range timeline {
			fmt.Printf("%d,%d,%d,%d\n", s.t.Unix(), s.node, s.created, s.ready)
		}
		os.Exit(0)
	}
}

type event struct {
	nodeCreation time.Time
	nncCreation  time.Time
	nncReady     time.Time
}

func (e event) nncCreateLatency() time.Duration {
	if e.nodeCreation.IsZero() || e.nncCreation.IsZero() {
		return -1
	}
	return e.nncCreation.Sub(e.nodeCreation)
}

func (e event) nncReadyLatency() time.Duration {
	if e.nncCreation.IsZero() || e.nncReady.IsZero() {
		return -1
	}
	return e.nncReady.Sub(e.nncCreation)
}

func (e event) created() bool {
	return !e.nncCreation.IsZero()
}

func (e event) ready() bool {
	return !e.nncReady.IsZero()
}
