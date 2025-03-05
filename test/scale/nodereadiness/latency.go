package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	nnc = schema.GroupVersionResource{
		Group:    "acn.azure.com",
		Version:  "v1alpha",
		Resource: "nodenetworkconfigs",
	}

	creationLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nnc_creation_latency",
		Help:    "Latency between NNC added and created",
		Buckets: prometheus.DefBuckets,
	}, []string{"todo"})

	// node name -> node creationTimestamp
	nodeCreation = make(map[string]time.Time)
	nncCreation  = make(map[string]time.Time)
)

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

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	//prometheus.MustRegister(creationLatency)

	// select {
	// case <-context.Background().Done():
	// 	fmt.Println("context done")
	// default:

	// }

	wg := sync.WaitGroup{}
	wg.Add(2)
	go watchNodes(clientset, &wg)
	go watchNNC(dynamicClient, &wg)

	wg.Wait()
	//avg := summarizeLatency(addedTime, createdTime)
	//fmt.Printf("Average latency (s): %v\n", avg)

	// fmt.Printf("hey\n")
	// http.Handle("/metrics", promhttp.Handler())
	// http.ListenAndServe(":2112", nil)
}

func watchNodes(clientset *kubernetes.Clientset, wg *sync.WaitGroup) {
	defer wg.Done()

	nodeWatcher, err := clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for event := range nodeWatcher.ResultChan() {
		switch event.Type {
		case watch.Added:
			fmt.Printf("Node added: %v at %v \n", event.Object.(*corev1.Node).Name, event.Object.(*corev1.Node).CreationTimestamp)
			// todo: record node added timestamp
			nodeCreation[event.Object.(*corev1.Node).Name] = event.Object.(*corev1.Node).CreationTimestamp.Time
		case watch.Modified:
		case watch.Deleted:
		case watch.Error:
		case watch.Bookmark:
		}
	}
}

func watchNNC(dynamicClient *dynamic.DynamicClient, wg *sync.WaitGroup) {
	defer wg.Done()

	// TODO: Should we skip cache syncing on start up?
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(nnc).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nncCreation[obj.(*unstructured.Unstructured).GetName()] = obj.(*unstructured.Unstructured).GetCreationTimestamp().Time
			fmt.Printf("NNC created: %v at %v\n", obj.(*unstructured.Unstructured).GetName(), obj.(*unstructured.Unstructured).GetCreationTimestamp().Time)
			// TODO: update Prometheus metric here
		},
		UpdateFunc: func(oldObj, newObj interface{}) {},
		DeleteFunc: func(obj interface{}) {},
	})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	informer.Run(ctx.Done())
}
