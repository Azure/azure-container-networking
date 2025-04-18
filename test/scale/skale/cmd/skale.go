// This code generates KWOK Nodes for a scale test of Swift controlplane components.
// It creates the Nodes and records metrics to measure the performance.
package cmd

import (
	"net/netip"
	"os"

	"github.com/Azure/azure-container-networking/test/scale/skale/pkg/nodes"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	z        *zap.Logger
	kubecli  *kubernetes.Clientset
	dynacli  dynamic.Interface
	rootopts = struct {
		genericclioptions.ConfigFlags
		subnet     string
		subnetGUID string
		vnetGUID   string
		nodes      int
		nodesubnet string
		pods       int
		cleanup    bool
		mode       string
		loglevel   string
		ratelimit  int
	}{}
)

var Skale = &cobra.Command{
	Use:               "skale",
	Short:             "Run ACN scale test",
	PersistentPreRunE: setup,
}

var Up = &cobra.Command{
	Use:   "up",
	Short: "Create nodes",
	Run: func(cmd *cobra.Command, args []string) {
		nodes.Add(cmd.Context(), z, kubecli, &nodes.AddOptions{
			Subnet:      netip.MustParsePrefix(rootopts.subnet),
			SubnetGUID:  rootopts.subnetGUID,
			VnetGUID:    rootopts.vnetGUID,
			Count:       rootopts.nodes,
			Mode:        nodes.Mode(rootopts.mode),
			Pods:        rootopts.pods,
			Concurrency: rootopts.ratelimit,
		})
	},
}

var Down = &cobra.Command{
	Use:   "down",
	Short: "Delete nodes",
	Run: func(cmd *cobra.Command, args []string) {
		nodes.Delete(cmd.Context(), z, kubecli, &nodes.DeleteOptions{Count: rootopts.nodes, Concurrency: rootopts.ratelimit})
	},
}

func init() {
	rootopts.ConfigFlags = *genericclioptions.NewConfigFlags(true)
	Skale.PersistentFlags().StringVar(&rootopts.loglevel, "log-level", "debug", "Log level")
	Skale.PersistentFlags().IntVar(&rootopts.nodes, "count", 10, "Number of nodes to modify")
	Skale.PersistentFlags().IntVar(&rootopts.ratelimit, "rate-limit", 500, "Rate limit for the client and Node creation")
	rootopts.AddFlags(Skale.PersistentFlags())
	Up.Flags().StringVar(&rootopts.mode, "mode", "overlay", "Set the CNI mode of the nodes (overlay, vnetblock, podsubnet)")
	Up.Flags().StringVar(&rootopts.subnet, "subnet", "10.255.0.0/16", "Subnet to use for the nodes")
	Up.Flags().StringVar(&rootopts.subnetGUID, "subnet-guid", "", "Subnet GUID to use for the nodes")
	Up.Flags().StringVar(&rootopts.vnetGUID, "vnet-guid", "", "VNet GUID to use for the nodes")
	Up.Flags().StringVar(&rootopts.nodesubnet, "node-subnet", "", "Subnet to use for the nodes")
	Up.Flags().BoolVar(&rootopts.cleanup, "cleanup", false, "Cleanup nodes after test")
	Up.Flags().IntVar(&rootopts.pods, "max-pods", 250, "Max pods per node")
	Skale.AddCommand(Up, Down)
}

func setup(*cobra.Command, []string) error {
	kubeConfig, err := ctrl.GetConfig()
	kubeConfig.QPS = float32(rootopts.ratelimit)
	kubeConfig.Burst = rootopts.ratelimit * 2
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
	z = zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zcfg), os.Stdout, lvl)) //.With(zap.String("cluster", kubeConfig.Host))
	return nil
}
