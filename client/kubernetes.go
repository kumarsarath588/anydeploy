package provisioner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	TIMEOUT = 300
)

func int32Ptr(i int32) *int32 { return &i }

func CreateKubernetesDeployment(uuid string, name string, containers []*Container) error {

	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)

	var dep_containers []apiv1.Container

	for _, container := range containers {
		var dep_ports []apiv1.ContainerPort
		for _, port := range container.Ports {
			dep_port := apiv1.ContainerPort{
				Name:          port.Name,
				Protocol:      apiv1.ProtocolTCP,
				ContainerPort: int32(port.Port),
			}
			dep_ports = append(dep_ports, dep_port)
		}
		dep_container := apiv1.Container{
			Name:  container.Name,
			Image: fmt.Sprintf("%s:%s", container.Image, container.Tag),
			Ports: dep_ports,
		}
		dep_containers = append(dep_containers, dep_container)
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  name,
					"uuid": uuid,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":  name,
						"uuid": uuid,
					},
				},
				Spec: apiv1.PodSpec{
					Containers: dep_containers,
				},
			},
		},
	}

	// Create Deployment
	result, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	log.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())
	for start := time.Now(); time.Since(start) < (time.Second * TIMEOUT); {
		get, err := deploymentsClient.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Creating Deployment failed %s", err.Error())
			return err
		}
		if get.Status.Replicas == get.Status.AvailableReplicas {
			for _, condition := range get.Status.Conditions {
				if condition.Type == "Available" && condition.Status == "True" {
					log.Printf("Success deployment %s : %q.\n", name, condition.Message)
					return nil
				}
			}
		}
		time.Sleep(time.Second * 10)
	}

	return fmt.Errorf("deployment %s failed", name)
}

func CreateKubernetesPublishedService(uuid string, name string, spec *ProvisionSpec) (string, error) {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return "", err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	serviceClient := clientset.CoreV1().Services(apiv1.NamespaceDefault)
	serviceName := fmt.Sprintf("%s-service", name)

	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
			Labels: map[string]string{
				"app":  name,
				"uuid": uuid,
			},
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"app":  name,
				"uuid": uuid,
			},
			Type: apiv1.ServiceType(spec.PublishedServiceType),
			Ports: []apiv1.ServicePort{
				{
					Port: int32(spec.Expose),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(spec.Expose),
					},
				},
			},
		},
	}

	// Create Service
	log.Printf("Creating Published service %s", serviceName)
	result, err := serviceClient.Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	for start := time.Now(); time.Since(start) < (time.Second * TIMEOUT); {
		get, err := serviceClient.Get(context.TODO(), result.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("Creating Published service failed %s", err.Error())
			return "", err
		}
		if spec.PublishedServiceType == "ClusterIP" {
			if clusterIP := get.Spec.ClusterIP; clusterIP != "" {
				log.Printf("Successfully fetched Published service Cluster IP")
				return clusterIP, nil
			}
		} else if spec.PublishedServiceType == "LoadBalancer" {
			if len(get.Status.LoadBalancer.Ingress) > 0 {
				if externalIP := get.Status.LoadBalancer.Ingress[0].IP; externalIP != "" {
					log.Printf("Successfully fetched Published service Loadbalancer IP")
					return externalIP, nil
				}
			}
		} else {
			log.Printf("Published service type is not enabled")
			return "", nil
		}
		time.Sleep(time.Second * 10)
	}

	return "", nil

}

func DeleteKubernetesDeployment(uuid string, name string) error {

	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)

	// Validation before deletion can be implemented
	// get, err := deploymentsClient.Get(context.TODO(), name, metav1.GetOptions{})

	// Delete Deployment
	log.Printf("Deleting deployment %s", name)
	err = deploymentsClient.Delete(context.TODO(), name, metav1.DeleteOptions{})

	if err != nil {
		log.Printf("Deleting Deployment failed %s", err.Error())
		return err
	}
	log.Printf("Deleted Deployment %s", name)
	return nil
}

func DeleteKubernetesPublishedService(uuid string, name string) error {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	serviceClient := clientset.CoreV1().Services(apiv1.NamespaceDefault)
	serviceName := fmt.Sprintf("%s-service", name)

	// Validation before deletion can be implemented
	//get, err := serviceClient.Get(context.TODO(), name, metav1.GetOptions{})
	//if err != nil {
	//	return err
	//}

	// Delete Service
	log.Printf("Deleting published service %s", serviceName)
	err = serviceClient.Delete(context.TODO(), serviceName, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Deleting published service failed %s", err.Error())
		return err
	}
	log.Printf("Deleted published service %s", serviceName)

	return nil
}
