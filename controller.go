package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	fmt.Println("worker called")
	for c.processItem() {

	}

}

func (c *controller) processItem() bool {
	fmt.Println("item processing")
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Forget(item)
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

	//check if the object has been deleted from k8s cluster
	ctx := context.Background()
	_, err = c.clientSet.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Printf("handle delete event for deployment %s \n", name)

		err = c.clientSet.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("Error in deleting service %s\n", err.Error())
		}
		fmt.Printf("Deleted service %s\n", name)

		err = c.clientSet.NetworkingV1().Ingresses(ns).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("Error in deleting ingress %s\n", err.Error())
		}
		fmt.Printf("Deleted ingress %s\n", name)
		return true
	}

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
	// fmt.Printf("dep var from lister %s\n", dep)
	if err != nil {
		fmt.Printf("Getting deployment from lister %s\n", err.Error())
	}

	//create service
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			//name will be same as deployment
			Name:      dep.Name,
			Namespace: ns,
			Labels:    dep.Spec.Template.Labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: dep.Spec.Template.Labels,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
				},
			},
		},
	}
	s, err := c.clientSet.CoreV1().Services(ns).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("creating service %s\n", err.Error())
	}

	//create ingress
	return createIngress(ctx, c.clientSet, s)

	return nil
}

func createIngress(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	PathType := "Prefix"
	ingress := netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				netv1.IngressRule{
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								netv1.HTTPIngressPath{
									Path:     "/" + svc.Name,
									PathType: (*netv1.PathType)(&PathType),
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: svc.Name,
											Port: netv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := client.NetworkingV1().Ingresses(svc.Namespace).Create(ctx, &ingress, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Creating ingress %s", err.Error())
		return err
	}
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
