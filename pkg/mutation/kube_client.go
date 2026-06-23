package mutation

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubeClient abstracts Kubernetes reads needed for resource injection.
type KubeClient interface {
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
	GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error)
}

type kubeClient struct {
	client kubernetes.Interface
}

// NewKubeClient returns a client backed by in-cluster configuration.
func NewKubeClient() (KubeClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &kubeClient{client: client}, nil
}

func (k *kubeClient) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

func (k *kubeClient) GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error) {
	return k.client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
}
