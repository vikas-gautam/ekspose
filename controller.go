package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	applisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	clientSet      kubernetes.Interface
	depLister      applisters.DeploymentLister
	depCacheSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
}

func newController(clientset kubernetes.Interface, depInformer appsinformers.DeploymentInformer) *controller {
	c := &controller{
		clientSet:      clientset,
		depLister:      depInformer.Lister(),
		depCacheSynced: depInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ekspose"),
	}

	depInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleAdd,
			DeleteFunc: c.handleDel,
		},
	)
	return c
}

func (c *controller) run(ch <-chan struct{}) {
	fmt.Println("Starting controller")
	if !cache.WaitForCacheSync(ch, c.depCacheSynced) {
		fmt.Println("waiting for cache to be synced")
	}
	go wait.Until(c.worker, 1*time.Second, ch)

	<-ch

}

func (c *controller) worker() {
	for c.processItem() {

	}
	fmt.Println("worker called")

}

func (c *controller) processItem() bool {
	fmt.Println("item processing")
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	key, err := cache.MetaNamespaceKeyFunc(item)
	if err != nil {
		fmt.Printf("getting key from cache %s\n", err.Error())
		return false
	}
	fmt.Printf("printing key %s\n", key)

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		fmt.Printf("splitting key into namespace and name %s\n", err.Error())
		return false
	}
	fmt.Printf("printing ns and name out of key %s, %s\n", ns, name)

	err = c.syncDeployment(ns, name)
	if err != nil {
		//retry
		fmt.Printf("syncing deployment %s\n", err.Error())
		return false
	}
	return true
}

func (c *controller) syncDeployment(ns, name string) error {
	ctx := context.Background()
	//get the name of dep from lister not from apiserver
	dep, err := c.depLister.Deployments(ns).Get(name)
	if err != nil {
		fmt.Printf("Getting deployment from lister %s\n", err.Error())
	}

	//create service
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			//name will be same as deployment
			Name:      dep.Name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}
	c.clientSet.CoreV1().Services(ns).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("creating service %s\n", err.Error())
	}

	//create ingress

	return nil
}

func (c *controller) handleAdd(obj interface{}) {

	fmt.Println("add was called")
	c.queue.Add(obj)

}

func (c *controller) handleDel(obj interface{}) {
	fmt.Println("del was called")
	c.queue.Add(obj)

}
