package main

import (
	"flag"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "/home/vikash/.kube/config", "location to your kubeconfig file")
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)

	if err != nil {
		fmt.Printf("error %s handling config from flags", err.Error())
		config, err = rest.InClusterConfig()
		if err != nil {
			fmt.Printf("error %s loading Inclusterconfig", err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("error %s creating clientset", err.Error())
	}

	fmt.Println(clientset)
}
