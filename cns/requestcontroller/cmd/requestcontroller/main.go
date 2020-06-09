// Get kubeconfig, make config, give it to both cns watcher and nodenetworkconfig

package main

import (
	"fmt"
	"io/ioutil"

	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller/controllers"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	//Service name
	name = "azure-cns-requestcontroller"
)

var (
	// Base scheme for this runtime, we will add the CRD scheme to this in init
	scheme = runtime.NewScheme()
)

// Version is populated by make during build.
var version string

// Command line arguments for CNS Request Controller.
var args = acn.ArgumentList{
	{
		Name:         acn.OptKubeConfigPath,
		Shorthand:    acn.OptKubeConfigPathAlias,
		Description:  "Set the kubeconfig file to use",
		Type:         "string",
		DefaultValue: "/var/lib/kubelet/kubeconfig",
	},
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

// add the CRD scheme to the runtime scheme
func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = nnc.AddToScheme(scheme)
}

func main() {
	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)

	//Get kube config path and logging settings from cmd line
	kubeconfigpath := acn.GetArg(acn.OptKubeConfigPath).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)
	logger.Printf("[cns-rc] Initialized logger for cns-requestcontroller")

	// Read the kubeconfig
	kubeconfigBytes, err := ioutil.ReadFile(kubeconfigpath)
	if err != nil {
		logger.Printf("[cns-rc] Error reading kubeconfig file given path %v. Error: %v", err)
		return
	}

	// Create a REST config from the kubeconfig file
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		logger.Printf("[cns-rc] Error creating REST config from kube config: %v", err)
		return
	}

	// Create manager for NodeNetworkConfigReconciler
	// MetricsBindAddress is the tcp address that the controller should bind to
	// for serving prometheus metrics, set to "0" to disable
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	if err != nil {
		logger.Printf("[cns-rc] Error creating new manager: %v", err)
		return
	}

	// Create NodeNetworkConfigReconciler
	nodeNetworkConfigController := &controllers.NodeNetworkConfigReconciler{
		Client: mgr.GetClient(),
	}

	// Setup manager with NodeNetworkConfigReconciler
	if err = nodeNetworkConfigController.SetupWithManager(mgr); err != nil {
		logger.Printf("[cns-rc] Error creating new NodeNetworkConfigController: %v", err)
		return
	}

	// Start manager and consequently, the controller
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Printf("[cns-rc] Error starting manager: %v", err)
		return
	}

	//TODO: Setup unix domain socket to listen for CNS publishes (cnslistener)
}
