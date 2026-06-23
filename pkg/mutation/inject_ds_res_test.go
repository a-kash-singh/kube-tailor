package mutation

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectDsResSkipsWithoutDaemonSetOwner(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "plain-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: &fakeKubeClient{}}.Mutate(pod)
	require.NoError(t, err)
	assert.Equal(t, pod, got)
}

func TestInjectDsResSkipsWhenNotOptedIn(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled: "false",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)
	assert.Equal(t, pod, got)
}

func TestInjectDsResAppliesCPUAndMemory(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel:    "2",
						karpenterInstanceMemoryLabel: "8192",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:       "true",
						LabelCPUPercent:    "10",
						LabelMemoryPercent: "5",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	resources := got.Spec.Containers[0].Resources
	assert.Equal(t, "200m", resources.Requests.Cpu().String())
	_, hasCPULimit := resources.Limits[corev1.ResourceCPU]
	assert.False(t, hasCPULimit)
	assert.Equal(t, "409Mi", resources.Requests.Memory().String())
	assert.Equal(t, "409Mi", resources.Limits.Memory().String())
}

func TestInjectDsResUsesDifferentPercentages(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel:    "4",
						karpenterInstanceMemoryLabel: "16384",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/metrics": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metrics",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:    "true",
						LabelCPUPercent: "5",
					},
				},
			},
		},
	}
	pod.OwnerReferences[0].Name = "metrics"

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	resources := got.Spec.Containers[0].Resources
	assert.Equal(t, "200m", resources.Requests.Cpu().String())
	_, hasMemory := resources.Requests[corev1.ResourceMemory]
	assert.False(t, hasMemory)
}

func TestInjectDsResClampsCPUToMin(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel: "2",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:    "true",
						LabelCPUPercent: "10",
						LabelCPUMin:     "500m",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)
	assert.Equal(t, "500m", got.Spec.Containers[0].Resources.Requests.Cpu().String())
}

func TestInjectDsResClampsCPUToMax(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel: "4",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:    "true",
						LabelCPUPercent: "10",
						LabelCPUMax:     "100m",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)
	assert.Equal(t, "100m", got.Spec.Containers[0].Resources.Requests.Cpu().String())
}

func TestInjectDsResIgnoresInvalidCPUBounds(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel: "2",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:    "true",
						LabelCPUPercent: "10",
						LabelCPUMin:     "500m",
						LabelCPUMax:     "100m",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	// 10% of 2 vCPU = 200m; invalid bounds are ignored so result stays 200m.
	assert.Equal(t, "200m", got.Spec.Containers[0].Resources.Requests.Cpu().String())
}

func TestInjectDsResClampsMemoryToMin(t *testing.T) {
	pod := daemonSetPod("worker-1")
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceMemoryLabel: "8192",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:       "true",
						LabelMemoryPercent: "5",
						LabelMemoryMin:     "512Mi",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)
	resources := got.Spec.Containers[0].Resources
	assert.Equal(t, "512Mi", resources.Requests.Memory().String())
	assert.Equal(t, "512Mi", resources.Limits.Memory().String())
}

func TestInjectDsResRemovesExistingCPULimit(t *testing.T) {
	pod := daemonSetPod("worker-1")
	pod.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("50m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("100m"),
		},
	}

	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel: "2",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:    "true",
						LabelCPUPercent: "10",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	resources := got.Spec.Containers[0].Resources
	assert.Equal(t, "200m", resources.Requests.Cpu().String())
	_, hasCPULimit := resources.Limits[corev1.ResourceCPU]
	assert.False(t, hasCPULimit)
}

func TestInjectDsResAppliesToAllContainersByDefault(t *testing.T) {
	pod := daemonSetPodWithContainers("worker-1", []string{"abc", "xyz"}, nil)
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel:    "2",
						karpenterInstanceMemoryLabel: "8192",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:       "true",
						LabelCPUPercent:    "10",
						LabelMemoryPercent: "5",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	for _, c := range got.Spec.Containers {
		resources := c.Resources
		assert.Equal(t, "200m", resources.Requests.Cpu().String())
		_, hasCPULimit := resources.Limits[corev1.ResourceCPU]
		assert.False(t, hasCPULimit)
		assert.Equal(t, "409Mi", resources.Requests.Memory().String())
		assert.Equal(t, "409Mi", resources.Limits.Memory().String())
	}
}

func TestInjectDsResTargetsOnlySpecifiedContainers(t *testing.T) {
	initialXYZ := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("25m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}

	pod := daemonSetPodWithContainers("worker-1", []string{"abc", "xyz"}, map[string]corev1.ResourceRequirements{
		"xyz": initialXYZ,
	})
	client := &fakeKubeClient{
		nodes: map[string]*corev1.Node{
			"worker-1": {
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Labels: map[string]string{
						karpenterInstanceCPULabel:    "2",
						karpenterInstanceMemoryLabel: "8192",
					},
				},
			},
		},
		daemonSets: map[string]*appsv1.DaemonSet{
			"apps/alpine": {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "alpine",
					Namespace: "apps",
					Labels: map[string]string{
						LabelEnabled:             "true",
						LabelCPUPercent:          "10",
						LabelMemoryPercent:       "5",
						LabelTargetedContainers:  "abc",
					},
				},
			},
		},
	}

	got, err := injectDsRes{Logger: logger(), Client: client}.Mutate(pod)
	require.NoError(t, err)

	var abcRes, xyzRes corev1.ResourceRequirements
	for _, c := range got.Spec.Containers {
		if c.Name == "abc" {
			abcRes = c.Resources
		}
		if c.Name == "xyz" {
			xyzRes = c.Resources
		}
	}

	// abc got updated
	assert.Equal(t, "200m", abcRes.Requests.Cpu().String())
	_, hasCPULimit := abcRes.Limits[corev1.ResourceCPU]
	assert.False(t, hasCPULimit)
	assert.Equal(t, "409Mi", abcRes.Requests.Memory().String())
	assert.Equal(t, "409Mi", abcRes.Limits.Memory().String())

	// xyz stayed unchanged
	assert.Equal(t, "25m", xyzRes.Requests.Cpu().String())
	_, hasCPULimitXYZ := xyzRes.Limits[corev1.ResourceCPU]
	assert.True(t, hasCPULimitXYZ)
	assert.Equal(t, "100m", xyzRes.Limits.Cpu().String())
	assert.Equal(t, "128Mi", xyzRes.Requests.Memory().String())
	assert.Equal(t, "256Mi", xyzRes.Limits.Memory().String())
}

func daemonSetPod(nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpine-abc",
			Namespace: "apps",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "DaemonSet",
				Name: "alpine",
			}},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name: "alpine",
			}},
		},
	}
}

func daemonSetPodWithContainers(nodeName string, containerNames []string, containerResources map[string]corev1.ResourceRequirements) *corev1.Pod {
	containers := make([]corev1.Container, 0, len(containerNames))
	for _, name := range containerNames {
		c := corev1.Container{Name: name}
		if containerResources != nil {
			if r, ok := containerResources[name]; ok {
				c.Resources = r
			}
		}
		containers = append(containers, c)
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpine-abc",
			Namespace: "apps",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "DaemonSet",
				Name: "alpine",
			}},
		},
		Spec: corev1.PodSpec{
			NodeName:   nodeName,
			Containers: containers,
		},
	}
}
