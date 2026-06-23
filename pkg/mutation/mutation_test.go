package mutation

import (
	"io/ioutil"
	"testing"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutatePodPatchNoOpForNonDaemonSet(t *testing.T) {
	m := NewMutatorWithClient(logger(), &fakeKubeClient{})
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "deploy-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	got, err := m.MutatePodPatch(pod)
	require.NoError(t, err)
	assert.True(t, string(got) == "[]" || string(got) == "null")
}

func TestMutatePodPatchInjectsResources(t *testing.T) {
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

	m := NewMutatorWithClient(logger(), client)
	got, err := m.MutatePodPatch(daemonSetPod("worker-1"))
	require.NoError(t, err)
	require.NotEqual(t, "[]", string(got))
}

func NewMutatorWithClient(logger *logrus.Entry, client KubeClient) *Mutator {
	return &Mutator{Logger: logger, Client: client}
}

func logger() *logrus.Entry {
	mute := logrus.StandardLogger()
	mute.Out = ioutil.Discard
	return mute.WithField("logger", "test")
}
