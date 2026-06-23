package mutation

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	karpenterInstanceCPULabel    = "karpenter.k8s.aws/instance-cpu"
	karpenterInstanceMemoryLabel = "karpenter.k8s.aws/instance-memory"
)

// NodeCapacity represents node resources used for percentage calculations.
type NodeCapacity struct {
	milliCPU *int64
	memoryMi *int64
}

// NodeCapacityFromNode resolves CPU (millicores) and memory (MiB) from node labels
// or status capacity as a fallback for non-Karpenter clusters.
func NodeCapacityFromNode(node *corev1.Node) (NodeCapacity, error) {
	if node == nil {
		return NodeCapacity{}, fmt.Errorf("node is nil")
	}

	capacity := NodeCapacity{}

	if cpuLabel, ok := node.Labels[karpenterInstanceCPULabel]; ok {
		vcpus, err := strconv.Atoi(cpuLabel)
		if err != nil {
			return NodeCapacity{}, fmt.Errorf("invalid %q label %q: %w", karpenterInstanceCPULabel, cpuLabel, err)
		}
		milliCPU := int64(vcpus) * 1000
		capacity.milliCPU = &milliCPU
	} else if cpuQty, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
		milliCPU := cpuQty.MilliValue()
		capacity.milliCPU = &milliCPU
	}

	if memLabel, ok := node.Labels[karpenterInstanceMemoryLabel]; ok {
		memoryMi, err := strconv.ParseInt(memLabel, 10, 64)
		if err != nil {
			return NodeCapacity{}, fmt.Errorf("invalid %q label %q: %w", karpenterInstanceMemoryLabel, memLabel, err)
		}
		capacity.memoryMi = &memoryMi
	} else if memQty, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
		memoryMi := memQty.Value() / (1024 * 1024)
		capacity.memoryMi = &memoryMi
	}

	return capacity, nil
}

// ValidateForConfig checks that the capacity has the data required by cfg.
// Call this immediately after NodeCapacityFromNode, before any calculation,
// so errors surface with a clear message rather than a generic "no capacity" error
// two frames deep into applyInjectionConfig.
func (c NodeCapacity) ValidateForConfig(cfg InjectionConfig) error {
	if cfg.CPUPercent != nil && c.milliCPU == nil {
		return fmt.Errorf(
			"cpu-percent is configured but node has no CPU capacity: "+
				"missing %q label and no status.capacity entry",
			karpenterInstanceCPULabel,
		)
	}
	if cfg.MemoryPercent != nil && c.memoryMi == nil {
		return fmt.Errorf(
			"memory-percent is configured but node has no memory capacity: "+
				"missing %q label and no status.capacity entry",
			karpenterInstanceMemoryLabel,
		)
	}
	return nil
}

func (c NodeCapacity) MilliCPUValue() (int64, error) {
	if c.milliCPU == nil {
		return 0, fmt.Errorf("node has no CPU capacity information")
	}
	return *c.milliCPU, nil
}

func (c NodeCapacity) MemoryMiValue() (int64, error) {
	if c.memoryMi == nil {
		return 0, fmt.Errorf("node has no memory capacity information")
	}
	return *c.memoryMi, nil
}

func cpuQuantityFromPercent(milliCPU int64, percent float64) resource.Quantity {
	millicores := int64(float64(milliCPU) * percent / 100)
	if millicores < 1 {
		millicores = 1
	}
	return resource.MustParse(fmt.Sprintf("%dm", millicores))
}

func memoryQuantityFromPercent(memoryMi int64, percent float64) resource.Quantity {
	memoryMiAllocated := int64(float64(memoryMi) * percent / 100)
	if memoryMiAllocated < 1 {
		memoryMiAllocated = 1
	}
	return resource.MustParse(fmt.Sprintf("%dMi", memoryMiAllocated))
}
