package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/workqueue"
)

var (
	nnc = schema.GroupVersionResource{
		Group:    "acn.azure.com",
		Version:  "v1alpha",
		Resource: "nodenetworkconfigs",
	}

	nncLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "nnc_creation_latency_seconds",
		Help: "Latency between NNC added and created",
		Buckets: []float64{0.05, 0.1, 0.5, 1.0, 1.5, 2, 3,
			4, 5, 6, 8, 10, 15, 20, 30, 45, 60, 120, 180, 240, 300, 450, 600, 900, 1200}, // WIP
	}, []string{"stage"})

	nncReadyCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nnc_ready",
		Help: "Number of NNCs that are ready",
	})

	nodeReadyCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "node_ready",
		Help: "Number of nodes that are ready",
	})
)

// WIP
type NNCController struct {
	dynamicClient dynamic.Interface
	workqueue     workqueue.TypedRateLimitingInterface[interface{}]

	clientset   *kubernetes.Clientset
	nodeWatcher watch.Interface

	nodeCreation map[string]time.Time
	nncCreation  map[string]time.Time
	nncReady     map[string]time.Time

	sync.RWMutex
}

func NewNNCController(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset) *NNCController {
	// TODO
	workqueue := workqueue.NewTypedRateLimitingQueue(
		workqueue.NewTypedItemExponentialFailureRateLimiter[interface{}](5*time.Millisecond, 1000*time.Second),
	)

	controller := &NNCController{
		dynamicClient: dynamicClient,
		workqueue:     workqueue,
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(nnc).Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addNNC,
		UpdateFunc: controller.updateNNC,
		DeleteFunc: func(obj interface{}) {},
	})

	nodeWatcher, err := clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	go func() {
		for event := range nodeWatcher.ResultChan() {
			switch event.Type {
			case watch.Added:
				name := event.Object.(*corev1.Node).Name
				timestamp := event.Object.(*corev1.Node).CreationTimestamp.Time
				if _, ok := controller.nodeCreation[name]; !ok {
					controller.nodeCreation[name] = timestamp
					fmt.Printf("Node added: %v at %v \n", name, timestamp)
				}
			case watch.Modified:
			case watch.Deleted:
			case watch.Error:
			case watch.Bookmark:
			}
		}
	}()

	return controller
}

func (c *NNCController) Run(ctx context.Context, workers int) {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	<-ctx.Done()
}

func (c *NNCController) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *NNCController) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		c.workqueue.AddRateLimited(obj)
		c.workqueue.Forget(obj)

		return nil
	}(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *NNCController) addNNC(obj interface{}) {
	name := obj.(*unstructured.Unstructured).GetName()
	timestamp := obj.(*unstructured.Unstructured).GetCreationTimestamp().Time
	if _, ok := c.nncCreation[name]; !ok {
		c.nncCreation[name] = timestamp
		fmt.Printf("NNC created: %v at %v \n", name, timestamp)
		if _, ok := c.nodeCreation[name]; ok {
			latency := c.nncCreation[name].Sub(c.nodeCreation[name])
			nncLatency.WithLabelValues("nodetonnc").Observe(latency.Seconds())
		}
	}
}

func (c *NNCController) updateNNC(oldObj, newObj interface{}) {
	if newObj.(*unstructured.Unstructured).Object != nil && newObj.(*unstructured.Unstructured).Object["status"] != nil {
		timestamp := time.Now() // probs not super accurate
		name := newObj.(*unstructured.Unstructured).GetName()
		if _, ok := c.nncReady[name]; !ok {
			c.nncReady[name] = timestamp
			fmt.Printf("NNC ready: %v at %v \n", name, timestamp)
			if _, ok := c.nncCreation[name]; ok {
				latency := c.nncReady[name].Sub(c.nncCreation[name])
				nncLatency.WithLabelValues("nncready").Observe(latency.Seconds())
				nncReadyCount.Inc()
			}
		}
	}
}

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

	ctx := context.Background()
	nncController := NewNNCController(ctx, dynamicClient, clientset)
	go nncController.Run(ctx, 20)

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}

// func watchNodes(clientset *kubernetes.Clientset, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	nodeWatcher, err := clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{})
// 	if err != nil {
// 		panic(err.Error())
// 	}

// 	for event := range nodeWatcher.ResultChan() {
// 		switch event.Type {
// 		case watch.Added:
// 			name := event.Object.(*corev1.Node).Name
// 			timestamp := event.Object.(*corev1.Node).CreationTimestamp.Time
// 			if _, ok := nodeCreation[name]; !ok {
// 				nodeCreation[name] = timestamp
// 				fmt.Printf("Node added: %v at %v \n", name, timestamp)
// 			}
// 		case watch.Modified:
// 		case watch.Deleted:
// 		case watch.Error:
// 		case watch.Bookmark:
// 		}
// 	}
// }

// func watchNNC(dynamicClient *dynamic.DynamicClient, wg *sync.WaitGroup) {
// 	defer wg.Done()

// 	// TODO: Should we skip cache syncing on start up? for now, initial node startup latencies are also counted
// 	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
// 	informer := factory.ForResource(nnc).Informer()

// 	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
// 		AddFunc: func(obj interface{}) {
// 			name := obj.(*unstructured.Unstructured).GetName()
// 			timestamp := obj.(*unstructured.Unstructured).GetCreationTimestamp().Time
// 			if _, ok := nncCreation[name]; !ok {
// 				nncCreation[name] = timestamp
// 				fmt.Printf("NNC created: %v at %v \n", name, timestamp)
// 				if _, ok := nodeCreation[name]; ok {
// 					latency := nncCreation[name].Sub(nodeCreation[name])
// 					nncLatency.WithLabelValues("nodetonnc").Observe(latency.Seconds())
// 				}
// 			}
// 		},
// 		UpdateFunc: func(oldObj, newObj interface{}) {
// 			if newObj.(*unstructured.Unstructured).Object != nil && newObj.(*unstructured.Unstructured).Object["status"] != nil {
// 				timestamp := time.Now() // probs not super accurate
// 				//nncStatus := newObj.(*unstructured.Unstructured).Object["status"]
// 				name := newObj.(*unstructured.Unstructured).GetName()
// 				if _, ok := nncReady[name]; !ok {
// 					nncReady[name] = timestamp
// 					fmt.Printf("NNC ready: %v at %v \n", name, timestamp)
// 					if _, ok := nncCreation[name]; ok {
// 						latency := nncReady[name].Sub(nncCreation[name])
// 						nncLatency.WithLabelValues("nncready").Observe(latency.Seconds())
// 						nncReadyCount.Inc()
// 					}
// 				}
// 			}
// 		},
// 		DeleteFunc: func(obj interface{}) {},
// 	})
// 	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
// 	defer cancel()

// 	informer.Run(ctx.Done())
// }
