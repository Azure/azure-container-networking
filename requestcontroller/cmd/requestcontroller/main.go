package main

import (
	"fmt"

	"github.com/Azure/azure-container-networking/cns/logger"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/requestcontroller/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

const name = "azure-cns-requestcontroller"
const k8sNamespace = "kube-system"

// Version is populated by make during build.
var version string

// Command line arguments for CNS Request Controller.
var args = acn.ArgumentList{
	{
		Name:         acn.OptLogLevel,
		Shorthand:    acn.OptLogLevelAlias,
		Description:  "Set the logging level",
		Type:         "int",
		DefaultValue: acn.OptLogLevelInfo,
		ValueMap: map[string]interface{}{
			acn.OptLogLevelInfo:  log.LevelInfo,
			acn.OptLogLevelDebug: log.LevelDebug,
		},
	},
	{
		Name:         acn.OptLogTarget,
		Shorthand:    acn.OptLogTargetAlias,
		Description:  "Set the logging target",
		Type:         "int",
		DefaultValue: acn.OptLogStdout,
		ValueMap: map[string]interface{}{
			acn.OptLogTargetSyslog: log.TargetSyslog,
			acn.OptLogTargetStderr: log.TargetStderr,
			acn.OptLogTargetFile:   log.TargetLogfile,
			acn.OptLogStdout:       log.TargetStdout,
			acn.OptLogMultiWrite:   log.TargetStdOutAndLogFile,
		},
	},
	{
		Name:         acn.OptLogLocation,
		Shorthand:    acn.OptLogLocationAlias,
		Description:  "Set the directory location where logs will be saved",
		Type:         "string",
		DefaultValue: "",
	},
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure CNS Request Controller\n")
	fmt.Printf("Version %v\n", version)
}

func main() {
	//Add CRD scheme to runtime sheme so manager can recognize it
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nnc.AddToScheme(scheme)

	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)
	logger.Printf("[cns-rc] Initialized logger for cns-requestcontroller")

	// Create manager for NodeNetworkConfigReconciler
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	// GetConfigOrDie precedence
	// * --kubeconfig flag pointing at a file at this cmd line
	// * KUBECONFIG environment variable pointing at a file
	// * In-cluster config if running in cluster
	// * $HOME/.kube/config if exists
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		Namespace:          k8sNamespace,
	})
	if err != nil {
		logger.Errorf("[cns-rc] Error creating new request controller manager: %v", err)
		return
	}

	// Create NodeNetworkConfigReconciler
	nodeNetworkConfigReconciler := &controllers.NodeNetworkConfigReconciler{
		K8sClient: mgr.GetClient(),
	}

	// Setup manager with NodeNetworkConfigReconciler
	if err = nodeNetworkConfigReconciler.SetupWithManager(mgr); err != nil {
		logger.Errorf("[cns-rc] Error creating new NodeNetworkConfigReconciler: %v", err)
		return
	}

	// Start manager and consequently, the reconciler
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Errorf("[cns-rc] Error starting manager: %v", err)
		return
	}

	//TODO: Setup unix domain socket to listen for CNS publishes (cnslistener)
}
