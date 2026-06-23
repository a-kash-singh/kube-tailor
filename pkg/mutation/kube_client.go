package mutation

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
)

// KubeClient abstracts Kubernetes reads needed for resource injection.
type KubeClient interface {
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
	GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error)
}

// kubeClient serves reads from an informer cache and falls back to a live API
// call on any cache error (including not-found, to handle tight create→admit
// races before the watch event arrives).
type kubeClient struct {
	client     kubernetes.Interface
	nodeLister corev1listers.NodeLister
	dsLister   appsv1listers.DaemonSetLister
}

// NewKubeClientFromConfig builds a KubeClient backed by shared informer caches.
// The caller must start the returned factory (factory.Start(stopCh)) and wait
// for cache sync (cache.WaitForCacheSync) before serving admission requests.
func NewKubeClientFromConfig(config *rest.Config) (KubeClient, informers.SharedInformerFactory, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	factory := informers.NewSharedInformerFactory(client, 0)

	// Pre-register the informers we need so factory.Start() picks them up.
	nodeLister := factory.Core().V1().Nodes().Lister()
	dsLister := factory.Apps().V1().DaemonSets().Lister()

	return &kubeClient{
		client:     client,
		nodeLister: nodeLister,
		dsLister:   dsLister,
	}, factory, nil
}

// NewKubeClient returns a client that makes live API calls on every request.
// Use this only for local development. In-cluster deployments should use
// NewKubeClientFromConfig with informer caches.
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
	if k.nodeLister != nil {
		node, err := k.nodeLister.Get(name)
		if err == nil {
			return node, nil
		}
		// On unexpected lister errors (not just not-found), fall through to a
		// live GET so a temporarily inconsistent cache doesn't block admission.
		if !apierrors.IsNotFound(err) {
			return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		}
		// Cache says not-found — still fall back to live GET to handle the race
		// where a brand-new node hasn't been reflected in the cache yet.
	}
	return k.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

func (k *kubeClient) GetDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error) {
	if k.dsLister != nil {
		ds, err := k.dsLister.DaemonSets(namespace).Get(name)
		if err == nil {
			return ds, nil
		}
		if !apierrors.IsNotFound(err) {
			return k.client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		}
	}
	return k.client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
}
