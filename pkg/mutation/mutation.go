package mutation

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
	"github.com/wI2L/jsondiff"
	corev1 "k8s.io/api/core/v1"
)

// Mutator is a container for mutation
type Mutator struct {
	Logger *logrus.Entry
	Client KubeClient
}

// NewMutator returns an initialised instance of Mutator
func NewMutator(logger *logrus.Entry) *Mutator {
	client, err := NewKubeClient()
	if err != nil {
		logger.WithError(err).Warn("failed to create kubernetes client; resource injection disabled")
	}

	return &Mutator{Logger: logger, Client: client}
}

// podMutators is an interface used to group functions mutating pods
type podMutator interface {
	Mutate(*corev1.Pod) (*corev1.Pod, error)
	Name() string
}

// MutatePodPatch returns a json patch containing all the mutations needed for
// a given pod
func (m *Mutator) MutatePodPatch(pod *corev1.Pod) ([]byte, error) {
	var podName string
	if pod.Name != "" {
		podName = pod.Name
	} else if pod.ObjectMeta.GenerateName != "" {
		podName = pod.ObjectMeta.GenerateName
	}
	log := m.Logger.WithField("pod_name", podName)

	mutations := []podMutator{
		injectDsRes{Logger: log, Client: m.Client},
	}

	mpod := pod.DeepCopy()

	for _, mutator := range mutations {
		var err error
		mpod, err = mutator.Mutate(mpod)
		if err != nil {
			return nil, err
		}
	}

	patch, err := jsondiff.Compare(pod, mpod)
	if err != nil {
		return nil, err
	}

	patchb, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	return patchb, nil
}
