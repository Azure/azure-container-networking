// This code generates KWOK Nodes for a scale test of Swift controlplane components.
// It creates the Nodes and records metrics to measure the performance.
package main

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	z       *zap.Logger
	kubecli *kubernetes.Clientset
	dynacli dynamic.Interface
	rootcmd = &cobra.Command{
		Use:   "skale",
		Short: "Run ACN scale test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context())
		},
		PersistentPreRunE: setup,
	}
	rootopts = struct {
		genericclioptions.ConfigFlags
		subnet     string
		subnetGUID string
		vnetGUID   string
		nodes      int
		nodesubnet string
		pods       int
		cleanup    bool
		overlay    bool
		vnetblock  bool
		loglevel   string
	}{}
)

func init() {
	rootopts.ConfigFlags = *genericclioptions.NewConfigFlags(true)
	rootcmd.PersistentFlags().StringVar(&rootopts.loglevel, "log-level", "debug", "Log level")
	rootopts.AddFlags(rootcmd.PersistentFlags())
	rootcmd.Flags().BoolVar(&rootopts.overlay, "overlay", false, "Set overlay labels on nodes")
	rootcmd.Flags().BoolVar(&rootopts.vnetblock, "vnet-block", false, "Set vnet block labels on nodes")
	rootcmd.Flags().StringVar(&rootopts.subnet, "subnet", "", "Subnet to use for the nodes")
	rootcmd.Flags().StringVar(&rootopts.subnetGUID, "subnet-guid", "", "Subnet GUID to use for the nodes")
	rootcmd.Flags().StringVar(&rootopts.vnetGUID, "vnet-guid", "", "VNet GUID to use for the nodes")
	rootcmd.Flags().IntVar(&rootopts.nodes, "nodes", 10, "Number of nodes to create")
	rootcmd.Flags().StringVar(&rootopts.nodesubnet, "node-subnet", "", "Subnet to use for the nodes")
	rootcmd.Flags().BoolVar(&rootopts.cleanup, "cleanup", false, "Cleanup nodes after test")
	rootcmd.Flags().IntVar(&rootopts.pods, "max-pods", 250, "Max pods per node")
}

func setup(*cobra.Command, []string) error {
	kubeConfig, err := ctrl.GetConfig()
	kubeConfig.QPS = 1000
	if err != nil {
		return errors.Wrap(err, "failed to get kubeconfig")
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build clientset")
	}
	kubecli = clientset
	d, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build dynamic client")
	}
	dynacli = d
	zcfg := zap.NewProductionEncoderConfig()
	zcfg.EncodeTime = zapcore.ISO8601TimeEncoder
	zcfg.EncodeDuration = zapcore.StringDurationEncoder
	lvl, _ := zapcore.ParseLevel(rootopts.loglevel)
	z = zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zcfg), os.Stdout, lvl)).With(zap.String("cluster", kubeConfig.Host))
	return nil
}

func run(ctx context.Context) error {
	z.Debug("starting with opts", zap.String("subnet", rootopts.subnet), zap.String("subnetGUID", rootopts.subnetGUID), zap.Int("nodes", rootopts.nodes))
	fakeNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kwok.x-k8s.io/node": "fake",
			},
			Labels: map[string]string{
				"type": "kwok",
				"kubernetes.azure.com/podnetwork-max-pods": strconv.Itoa(rootopts.pods),
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
	if rootopts.overlay {
		fakeNode.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-type"] = "overlay"
		fakeNode.ObjectMeta.Labels["kubernetes.azure.com/azure-cni-overlay"] = "true"
		fakeNode.ObjectMeta.Labels["kubernetes.azure.com/nodenetwork-vnetguid"] = rootopts.vnetGUID
	} else {
		if rootopts.vnetblock {
			fakeNode.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-type"] = "vnetblock"
		}
		fakeNode.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-delegationguid"] = rootopts.subnetGUID
		fakeNode.ObjectMeta.Labels["kubernetes.azure.com/podnetwork-subnet"] = rootopts.subnet
	}

	if !rootopts.cleanup {
		prefix, err := netip.ParsePrefix(rootopts.nodesubnet)
		if err != nil {
			return errors.Wrap(err, "failed to parse nodesubnet prefix")
		}
		nodeip := prefix.Masked().Addr()
		wg := sync.WaitGroup{}
		for i := range rootopts.nodes {
			wg.Add(1)
			fakeNode.Name = "skale-" + strconv.Itoa(i)
			go func(node corev1.Node, ip string) {
				_, err := kubecli.CoreV1().Nodes().Create(ctx, &node, metav1.CreateOptions{})
				if err != nil && !k8serr.IsAlreadyExists(err) {
					z.Error("failed to create node", zap.Error(err))
				}
				_, err = kubecli.CoreV1().Nodes().PatchStatus(ctx, node.Name, []byte(`{"status":{"addresses":[{"type":"InternalIP","address":"`+ip+`"}]}}`))
				if err != nil {
					z.Error("failed to patch node status", zap.Error(err))
				}
				z.Debug("created node", zap.String("name", node.Name))
				wg.Done()
			}(*fakeNode, nodeip.String())
			nodeip = nodeip.Next()
		}
		wg.Wait()
		z.Info("created nodes")
		return nil
	}
	// TODO: this is where we will put the tests
	wg := sync.WaitGroup{}
	for i := range rootopts.nodes {
		wg.Add(1)
		go func(i int) {
			name := "skale-" + strconv.Itoa(i)
			if err := kubecli.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !k8serr.IsNotFound(err) {
				z.Error("failed to delete node", zap.Error(err))
			}
			z.Debug("deleted node", zap.String("name", name))
			wg.Done()
		}(i)
	}
	wg.Wait()
	z.Info("deleted nodes")
	return nil
}
