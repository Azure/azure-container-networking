package nodes

import (
	"context"
	"net/netip"
	"strconv"
	"sync"

	"github.com/avast/retry-go/v4"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type limiter chan struct{}

func (l limiter) Wait() {
	l <- struct{}{}
}

func (l limiter) Done() {
	select {
	case <-l:
	default:
	}
}

type Mode string

const (
	Overlay   Mode = "overlay"
	VnetBlock Mode = "vnetblock"
	Podsubnet Mode = "podsubnet"
)

func templateNode(opts *AddOptions) *corev1.Node {
	tmpl := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kwok.x-k8s.io/node": "fake",
			},
			Labels: map[string]string{
				"type": "kwok",
				"kubernetes.azure.com/podnetwork-max-pods": strconv.Itoa(opts.Pods),
				"topology.kubernetes.io/zone":              "0",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    "kwok.x-k8s.io/node",
					Value:  "fake",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
	}
	if opts.Mode == Overlay {
		tmpl.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-type"] = "overlay"
		tmpl.ObjectMeta.Labels["kubernetes.azure.com/azure-cni-overlay"] = "true"
		tmpl.ObjectMeta.Labels["kubernetes.azure.com/nodenetwork-vnetguid"] = opts.VnetGUID
	} else {
		if opts.Mode == VnetBlock {
			tmpl.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-type"] = "vnetblock"
		}
		tmpl.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-delegationguid"] = opts.SubnetGUID
		tmpl.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-subnet"] = opts.Subnet.String()
	}
	return tmpl
}

type AddOptions struct {
	Subnet      netip.Prefix
	SubnetGUID  string
	VnetGUID    string
	Count       int
	Mode        Mode
	Pods        int
	Concurrency int
}

func Add(ctx context.Context, z *zap.Logger, kubecli *kubernetes.Clientset, opts *AddOptions) {
	fakeNode := templateNode(opts)
	nodeip := opts.Subnet.Masked().Addr()
	var pool limiter = make(chan struct{}, opts.Concurrency) // the buffered channel gates the number of concurrent goroutines
	wg := sync.WaitGroup{}
	for i := range opts.Count {
		wg.Add(1)
		fakeNode.Name = "skale-" + strconv.Itoa(i)
		go func(node corev1.Node, ip string) {
			pool.Wait()       // wait for a slot to be available
			defer pool.Done() // release the slot when done
			retry.Do(func() error {
				_, err := kubecli.CoreV1().Nodes().Create(ctx, &node, metav1.CreateOptions{})
				if err != nil && !k8serr.IsAlreadyExists(err) {
					z.Error("failed to create node", zap.Error(err))
					return err
				}
				return nil
			}, retry.UntilSucceeded())
			retry.Do(func() error {
				_, err := kubecli.CoreV1().Nodes().PatchStatus(ctx, node.Name, []byte(`{"status":{"addresses":[{"type":"InternalIP","address":"`+ip+`"}]}}`))
				if err != nil {
					z.Error("failed to patch node status", zap.Error(err))
					return err
				}
				return nil
			}, retry.UntilSucceeded())
			z.Debug("created node", zap.String("name", node.Name))
			wg.Done()
		}(*fakeNode, nodeip.String())
		nodeip = nodeip.Next()
	}
	wg.Wait()
	z.Info("created nodes")
}

type DeleteOptions struct {
	Count       int
	Concurrency int
}

func Delete(ctx context.Context, z *zap.Logger, kubecli *kubernetes.Clientset, opts *DeleteOptions) {
	var pool limiter = make(chan struct{}, opts.Concurrency) // the buffered channel gates the number of concurrent goroutines
	wg := sync.WaitGroup{}
	for i := range opts.Count {
		wg.Add(1)
		name := "skale-" + strconv.Itoa(i)
		go func(name string) {
			pool.Wait()       // wait for a slot to be available
			defer pool.Done() // release the slot when done
			retry.Do(func() error {
				if err := kubecli.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serr.IsNotFound(err) {
					z.Error("failed to delete node", zap.Error(err))
					return err
				}
				return nil
			}, retry.UntilSucceeded())
			z.Debug("deleted node", zap.String("name", name))
			wg.Done()
		}(name)
	}
	wg.Wait()
	z.Info("deleted nodes")
}
