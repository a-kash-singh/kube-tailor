package mutation

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type fakeKubeClient struct {
	nodes       map[string]*corev1.Node
	daemonSets  map[string]*appsv1.DaemonSet
}

func (f *fakeKubeClient) GetNode(_ context.Context, name string) (*corev1.Node, error) {
	node, ok := f.nodes[name]
	if !ok {
		return nil, fmt.Errorf("node %q not found", name)
	}
	return node, nil
}

func (f *fakeKubeClient) GetDaemonSet(_ context.Context, namespace, name string) (*appsv1.DaemonSet, error) {
	key := namespace + "/" + name
	ds, ok := f.daemonSets[key]
	if !ok {
		return nil, fmt.Errorf("daemonset %q not found", key)
	}
	return ds, nil
}
