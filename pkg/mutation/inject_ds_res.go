package mutation

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	injectDsResMutatorName = "inject_ds_res"
)

// injectDsRes injects CPU and memory resources into DaemonSet pods based on
// opt-in labels configured on the owning DaemonSet.
type injectDsRes struct {
	Logger logrus.FieldLogger
	Client KubeClient
}

var _ podMutator = (*injectDsRes)(nil)

func (se injectDsRes) Name() string {
	return injectDsResMutatorName
}

func (se injectDsRes) Mutate(pod *corev1.Pod) (*corev1.Pod, error) {
	if se.Client == nil {
		return pod, fmt.Errorf("kubernetes client is not configured")
	}

	dsRef, ok := daemonSetOwner(pod)
	if !ok {
		se.Logger.Debug("pod is not owned by a DaemonSet, skipping resource injection")
		return pod, nil
	}

	ds, err := se.Client.GetDaemonSet(context.Background(), dsRef.Namespace, dsRef.Name)
	if err != nil {
		return nil, fmt.Errorf("fetch daemonset %s/%s: %w", dsRef.Namespace, dsRef.Name, err)
	}

	labels := MergeLabelMaps(ds.Labels, ds.Spec.Template.Labels)
	cfg, err := ParseInjectionConfig(labels)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		se.Logger.WithField("daemonset", dsRef.Name).Debug("daemonset not opted in, skipping resource injection")
		return pod, nil
	}

	applyAllContainers := len(cfg.TargetedContainers) == 0
	var targeted map[string]struct{}
	if !applyAllContainers {
		targeted = make(map[string]struct{}, len(cfg.TargetedContainers))
		for _, n := range cfg.TargetedContainers {
			targeted[n] = struct{}{}
		}
		se.Logger.WithField("daemonset", dsRef.Name).
			Debugf("applying resource changes only to targeted containers: %v", cfg.TargetedContainers)
	} else {
		se.Logger.WithField("daemonset", dsRef.Name).
			Debug("applying resource changes to all available containers")
	}

	// If bounds are invalid (min > max), ignore them and continue without clamping.
	// We log this to help catch configuration mistakes without blocking pod creation.
	if err := validateMinMax(cfg.CPUMin, cfg.CPUMax, "cpu"); err != nil {
		se.Logger.WithError(err).Warnf("invalid cpu min/max bounds (%s=%v %s=%v); ignoring cpu bounds",
			LabelCPUMin, cfg.CPUMin, LabelCPUMax, cfg.CPUMax,
		)
		cfg.CPUMin = nil
		cfg.CPUMax = nil
	}
	if err := validateMinMax(cfg.MemoryMin, cfg.MemoryMax, "memory"); err != nil {
		se.Logger.WithError(err).Warnf("invalid memory min/max bounds (%s=%v %s=%v); ignoring memory bounds",
			LabelMemoryMin, cfg.MemoryMin, LabelMemoryMax, cfg.MemoryMax,
		)
		cfg.MemoryMin = nil
		cfg.MemoryMax = nil
	}

	nodeName, err := nodeNameFromPod(pod)
	if err != nil {
		return nil, err
	}

	node, err := se.Client.GetNode(context.Background(), nodeName)
	if err != nil {
		return nil, fmt.Errorf("fetch node %q: %w", nodeName, err)
	}

	capacity, err := NodeCapacityFromNode(node)
	if err != nil {
		return nil, err
	}

	se.Logger = se.Logger.WithFields(logrus.Fields{
		"mutation":   se.Name(),
		"daemonset":  dsRef.Name,
		"namespace":  dsRef.Namespace,
		"node":       nodeName,
	})

	mpod := pod.DeepCopy()
	for i := range mpod.Spec.Containers {
		if !applyAllContainers {
			if _, ok := targeted[mpod.Spec.Containers[i].Name]; !ok {
				continue
			}
		}
		requirements, err := applyInjectionConfig(cfg, capacity, mpod.Spec.Containers[i].Resources)
		if err != nil {
			return nil, fmt.Errorf("container %q: %w", mpod.Spec.Containers[i].Name, err)
		}
		mpod.Spec.Containers[i].Resources = requirements
		se.Logger.Debugf(
			"container %q resources: requests=%v limits=%v",
			mpod.Spec.Containers[i].Name,
			requirements.Requests,
			requirements.Limits,
		)
	}

	return mpod, nil
}

func applyInjectionConfig(
	cfg InjectionConfig,
	capacity NodeCapacity,
	current corev1.ResourceRequirements,
) (corev1.ResourceRequirements, error) {
	requirements := current.DeepCopy()
	if requirements.Requests == nil {
		requirements.Requests = corev1.ResourceList{}
	}
	if requirements.Limits == nil {
		requirements.Limits = corev1.ResourceList{}
	}

	if cfg.CPUPercent != nil {
		milliCPU, err := capacity.MilliCPUValue()
		if err != nil {
			return corev1.ResourceRequirements{}, err
		}
		cpu := cpuQuantityFromPercent(milliCPU, *cfg.CPUPercent)
		cpu = clampQuantity(cpu, cfg.CPUMin, cfg.CPUMax)
		requirements.Requests[corev1.ResourceCPU] = cpu
		delete(requirements.Limits, corev1.ResourceCPU)
	}

	if cfg.MemoryPercent != nil {
		memoryMi, err := capacity.MemoryMiValue()
		if err != nil {
			return corev1.ResourceRequirements{}, err
		}
		memory := memoryQuantityFromPercent(memoryMi, *cfg.MemoryPercent)
		memory = clampQuantity(memory, cfg.MemoryMin, cfg.MemoryMax)
		requirements.Requests[corev1.ResourceMemory] = memory
		requirements.Limits[corev1.ResourceMemory] = memory
	}

	return *requirements, nil
}

type daemonSetRef struct {
	Namespace string
	Name      string
}

func daemonSetOwner(pod *corev1.Pod) (daemonSetRef, bool) {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return daemonSetRef{
				Namespace: pod.Namespace,
				Name:      owner.Name,
			}, true
		}
	}
	return daemonSetRef{}, false
}

func nodeNameFromPod(pod *corev1.Pod) (string, error) {
	if pod.Spec.NodeName != "" {
		return pod.Spec.NodeName, nil
	}

	affinity := pod.Spec.Affinity
	if affinity == nil || affinity.NodeAffinity == nil {
		return "", fmt.Errorf("pod has no nodeName or node affinity required during scheduling")
	}

	required := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		return "", fmt.Errorf("pod has no required node affinity")
	}

	for _, term := range required.NodeSelectorTerms {
		for _, field := range term.MatchFields {
			if field.Key == metav1.ObjectNameField && len(field.Values) > 0 {
				return field.Values[0], nil
			}
		}
	}

	return "", fmt.Errorf("pod node affinity does not contain a node name match field")
}
