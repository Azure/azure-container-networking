package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// TODO:
// Switch panic to log.Fatal

// TODO: Temporary: need to get formatting requirements, Output to file? Prometheus metric?
func summarizeLatency(addedTime map[string]time.Time, createdTime map[string]time.Time) float64 {
	latencies := make(map[string]time.Duration)

	// This will currently skip nncs that were created before the informer started.
	// However, should probably have the informer skip those so we can assume such an nnc was never created (timed out?)
	// and track that.
	for nnc, addedTime := range addedTime {
		if _, ok := createdTime[nnc]; ok {
			latencies[nnc] = createdTime[nnc].Sub(addedTime)
		}
	}

	sum := time.Duration(0)
	for nnc, latency := range latencies {
		fmt.Printf("%v: %v\n", nnc, latency)
		sum += latency
	}
	return (float64(sum.Seconds()) / float64(len(latencies)))
}

func main() {
	// TODO: Allow user to pass kubeconfig arg.
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	nnc := schema.GroupVersionResource{
		Group:    "acn.azure.com",
		Version:  "v1alpha",
		Resource: "nodenetworkconfigs",
	}

	addedTime := make(map[string]time.Time)
	createdTime := make(map[string]time.Time)

	// TODO: Watch on test namespace?
	// TODO: Should we skip cache syncing on start up?
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(nnc).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			//fmt.Printf("Added: %v at %v\n", obj.(*unstructured.Unstructured).GetName(), time.Now())
			addedTime[obj.(*unstructured.Unstructured).GetName()] = time.Now()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			//fmt.Printf("Updated: %v at %v\n", newObj.(*unstructured.Unstructured).GetName(), newObj.(*unstructured.Unstructured).GetCreationTimestamp())
			createdTime[newObj.(*unstructured.Unstructured).GetName()] = newObj.(*unstructured.Unstructured).GetCreationTimestamp().Time
		},
		DeleteFunc: func(obj interface{}) {},
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	informer.Run(ctx.Done())
	avg := summarizeLatency(addedTime, createdTime)
	fmt.Printf("Average latency (s): %v\n", avg)
}
