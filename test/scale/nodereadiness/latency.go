package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	nnc = schema.GroupVersionResource{
		Group:    "acn.azure.com",
		Version:  "v1alpha",
		Resource: "nodenetworkconfigs",
	}

	nncLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "nnc_creation_latency",
		Help: "Latency between NNC added and created",
		Buckets: []float64{0.005, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1.0, 1.25, 1.5, 2, 3,
			4, 5, 6, 8, 10, 15, 20, 30, 45, 60}, // WIP
	}, []string{"stage"})

	nodeCreation = make(map[string]time.Time)
	nncCreation  = make(map[string]time.Time)
	nncReady     = make(map[string]time.Time)
)

func main() {
	// todo: Allow user to pass kubeconfig arg.
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

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

	prometheus.MustRegister(nncLatency)

	wg := sync.WaitGroup{} // todo
	wg.Add(2)
	go watchNodes(clientset, &wg)
	go watchNNC(dynamicClient, &wg)

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)

	wg.Wait()
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
			name := event.Object.(*corev1.Node).Name
			timestamp := event.Object.(*corev1.Node).CreationTimestamp.Time
			if _, ok := nodeCreation[name]; !ok {
				nodeCreation[name] = timestamp
				fmt.Printf("Node added: %v at %v \n", name, timestamp)
			}
		case watch.Modified:
		case watch.Deleted:
		case watch.Error:
		case watch.Bookmark:
		}
	}
}

func watchNNC(dynamicClient *dynamic.DynamicClient, wg *sync.WaitGroup) {
	defer wg.Done()

	// TODO: Should we skip cache syncing on start up? for now, initial node startup latencies are also counted
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(nnc).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			name := obj.(*unstructured.Unstructured).GetName()
			timestamp := obj.(*unstructured.Unstructured).GetCreationTimestamp().Time
			if _, ok := nncCreation[name]; !ok {
				nncCreation[name] = timestamp
				fmt.Printf("NNC created: %v at %v \n", name, timestamp)
				if _, ok := nodeCreation[name]; ok {
					latency := nncCreation[name].Sub(nodeCreation[name])
					nncLatency.WithLabelValues("nodetonnc").Observe(latency.Seconds())
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if newObj.(*unstructured.Unstructured).Object != nil && newObj.(*unstructured.Unstructured).Object["status"] != nil {
				timestamp := time.Now() // probs not super accurate
				//nncStatus := newObj.(*unstructured.Unstructured).Object["status"]
				name := newObj.(*unstructured.Unstructured).GetName()
				if _, ok := nncReady[name]; !ok {
					nncReady[name] = timestamp
					fmt.Printf("NNC ready: %v at %v \n", name, timestamp)
					if _, ok := nncCreation[name]; ok {
						latency := nncReady[name].Sub(nncCreation[name])
						nncLatency.WithLabelValues("nncready").Observe(latency.Seconds())
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {},
	})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	informer.Run(ctx.Done())
}
