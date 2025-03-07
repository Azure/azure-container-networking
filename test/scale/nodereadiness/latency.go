package main

import (
	"context"
	"log"
	"net/http"
	"strings"
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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
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

type NNCController struct {
	dynamicClient dynamic.Interface
	workqueue     workqueue.TypedRateLimitingInterface[interface{}]
	informer      cache.SharedIndexInformer

	clientset   *kubernetes.Clientset
	nodeWatcher watch.Interface

	nodeCreation map[string]time.Time
	nncCreation  map[string]time.Time
	nncReady     map[string]time.Time
	nodeReady    map[string]struct{}
	rcCreateNNC  map[string]time.Time
	dncRcStatus  map[string]time.Time

	sync.RWMutex
}

func NewNNCController(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	clientset *kubernetes.Clientset) *NNCController {
	workqueue := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[interface{}]())

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, nil)
	informer := factory.ForResource(nnc).Informer()

	nodeWatcher, err := clientset.CoreV1().Nodes().Watch(context.Background(), metav1.ListOptions{
		LabelSelector: "type=kwok",
	})
	if err != nil {
		panic(err.Error())
	}

	controller := &NNCController{
		dynamicClient: dynamicClient,
		workqueue:     workqueue,
		nodeCreation:  make(map[string]time.Time),
		nncCreation:   make(map[string]time.Time),
		nncReady:      make(map[string]time.Time),
		nodeReady:     make(map[string]struct{}),
		rcCreateNNC:   make(map[string]time.Time),
		dncRcStatus:   make(map[string]time.Time),
		clientset:     clientset,
		informer:      informer,
		nodeWatcher:   nodeWatcher,
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addNNC,
		UpdateFunc: controller.updateNNC,
		DeleteFunc: func(obj interface{}) {},
	})

	go func() { // TODO: Move to separate controller?
		for event := range nodeWatcher.ResultChan() {
			switch event.Type {
			case watch.Added:
				name := event.Object.(*corev1.Node).Name
				log.Printf("Node %v\n", name)
				timestamp := event.Object.(*corev1.Node).CreationTimestamp.Time
				if _, ok := controller.nodeCreation[name]; !ok {
					controller.nodeCreation[name] = timestamp
					log.Printf("Node added: %v at %v \n", name, timestamp)
				}
			case watch.Modified:
				name := event.Object.(*corev1.Node).Name
				log.Printf("Node %v\n", name)
				conditions := event.Object.(*corev1.Node).Status.Conditions
				for _, condition := range conditions {
					if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
						if _, ok := controller.nodeReady[name]; !ok {
							controller.nodeReady[name] = struct{}{}
							nodeReadyCount.Inc()
							log.Printf("Node ready: %v\n", name)
							continue
						}
					}
				}
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

	c.informer.Run(ctx.Done())

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
	log.Printf("NNC added: %v\n", obj.(*unstructured.Unstructured).GetName())
	name := obj.(*unstructured.Unstructured).GetName()
	timestamp := obj.(*unstructured.Unstructured).GetCreationTimestamp().Time
	if !isKwok(name) {
		return
	}
	if _, ok := c.nncCreation[name]; !ok {
		c.nncCreation[name] = timestamp
		log.Printf("NNC created: %v at %v \n", name, timestamp)
		if _, ok := c.nodeCreation[name]; ok {
			latency := c.nncCreation[name].Sub(c.nodeCreation[name])
			nncLatency.WithLabelValues("nodetonnc").Observe(latency.Seconds())
		}
	}
}

func (c *NNCController) updateNNC(oldObj, newObj interface{}) {
	log.Printf("NNC updated: %v\n", newObj.(*unstructured.Unstructured).GetName())
	// NNC Status written, as observed (less accurate)
	if newObj.(*unstructured.Unstructured).Object != nil && newObj.(*unstructured.Unstructured).Object["status"] != nil {
		timestamp := time.Now() // probs not super accurate
		name := newObj.(*unstructured.Unstructured).GetName()
		if !isKwok(name) {
			return
		}
		if _, ok := c.nncReady[name]; !ok {
			c.nncReady[name] = timestamp
			log.Printf("NNC ready: %v at %v \n", name, timestamp)
			if _, ok := c.nncCreation[name]; ok {
				latency := c.nncReady[name].Sub(c.nncCreation[name])
				log.Printf("NNC %v was created at %v and is ready at %v, with a latency of: %v\n", name, c.nncCreation[name], c.nncReady[name], latency.Seconds())
				nncLatency.WithLabelValues("nncready").Observe(latency.Seconds())
				nncReadyCount.Inc()
			}
		}
	}
	// Parse Managed Field Timestamps
	managedFields := newObj.(*unstructured.Unstructured).GetManagedFields()
	if managedFields != nil {
		for _, field := range managedFields {
			if field.Manager == "dnc-rc" && field.Operation == "Update" {
				timestamp := field.Time
				name := newObj.(*unstructured.Unstructured).GetName()
				if field.Subresource == "status" {
					if _, ok := c.dncRcStatus[name]; !ok {
						c.dncRcStatus[name] = timestamp.Time
						latency := c.dncRcStatus[name].Sub(c.rcCreateNNC[name])
						nncLatency.WithLabelValues("createToStatus").Observe(latency.Seconds())
					}
				} else {
					if _, ok := c.rcCreateNNC[name]; !ok {
						c.rcCreateNNC[name] = timestamp.Time
						latency := c.rcCreateNNC[name].Sub(c.nncCreation[name])
						nncLatency.WithLabelValues("rcCreate").Observe(latency.Seconds())
					}
				}
			}
		}
	}
}

func isKwok(name string) bool {
	return strings.Contains(name, "skale")
}

func main() {
	// create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	prometheus.MustRegister(nncLatency, nncReadyCount, nodeReadyCount)

	ctx := context.Background()
	nncController := NewNNCController(ctx, dynamicClient, clientset)
	go nncController.Run(ctx, 20)

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":2112", nil)
}
