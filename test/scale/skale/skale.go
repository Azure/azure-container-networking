// This code generates KWOK Nodes for a scale test of Swift controlplane components.
// It creates the Nodes and records metrics to measure the performance.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	z       *zap.Logger
	kubecli *kubernetes.Clientset
	rootcmd = &cobra.Command{
		Use:   "skale",
		Short: "Run ACN scale test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context())
		},
		PersistentPreRunE: setup,
	}
	rootopts = genericclioptions.NewConfigFlags(true)
)

func init() {
	rootopts.AddFlags(rootcmd.PersistentFlags())
	rootcmd.Flags().String("subnet", "", "Subnet to use for the nodes")
	rootcmd.Flags().String("subnet-guid", "", "Subnet GUID to use for the nodes")
}

func setup(*cobra.Command, []string) error {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get kubeconfig")
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build clientset")
	}
	kubecli = clientset
	z, err = zap.NewDevelopment()
	if err != nil {
		return errors.Wrap(err, "failed to create logger")
	}
	return nil
}

func run(ctx context.Context) error {
	fakeNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "skale-node",
			Annotations: map[string]string{
				"kwok.x-k8s.io/node": "fake",
			},
			Labels: map[string]string{
				"type": "kwok",
				"kubernetes.azure.com/podnetwork-delegationguid": "cf649a07-6690-41ff-b9ef-a5be9582de4f",
				"kubernetes.azure.com/podnetwork-max-pods":       "63",
				"kubernetes.azure.com/podnetwork-subnet":         "pod-subnet",
				"topology.kubernetes.io/zone":                    "0",
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

	nodes := generateNodes(fakeNode, 10)
	for _, node := range nodes {
		if _, err := kubecli.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{}); err != nil && !k8serr.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed to create node")
		}
		z.Info("created node", zap.String("name", node.Name))
	}
	z.Info("created nodes")
	time.Sleep(10 * time.Second)
	for _, node := range nodes {
		if err := kubecli.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{}); err != nil {
			return errors.Wrap(err, "failed to delete node")
		}
		z.Info("deleted node", zap.String("name", node.Name))
	}
	z.Info("deleted nodes")
	return nil
}

func generateNodes(skel *corev1.Node, num int) []*corev1.Node {
	nodes := make([]*corev1.Node, num)
	for i := range num {
		node := *skel.DeepCopy()
		node.Name = fmt.Sprintf("%s-%d", node.Name, i)
		nodes[i] = &node
	}
	return nodes
}
