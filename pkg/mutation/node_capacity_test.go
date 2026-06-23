package mutation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pct(v float64) *float64 { return &v }

func TestNodeCapacityFromNode_KarpenterLabels(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				karpenterInstanceCPULabel:    "4",
				karpenterInstanceMemoryLabel: "8192",
			},
		},
	}
	cap, err := NodeCapacityFromNode(node)
	require.NoError(t, err)

	cpu, err := cap.MilliCPUValue()
	require.NoError(t, err)
	assert.Equal(t, int64(4000), cpu)

	mem, err := cap.MemoryMiValue()
	require.NoError(t, err)
	assert.Equal(t, int64(8192), mem)
}

func TestNodeCapacityFromNode_StatusCapacityFallback(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-2"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4096Mi"),
			},
		},
	}
	cap, err := NodeCapacityFromNode(node)
	require.NoError(t, err)

	cpu, err := cap.MilliCPUValue()
	require.NoError(t, err)
	assert.Equal(t, int64(2000), cpu)

	mem, err := cap.MemoryMiValue()
	require.NoError(t, err)
	assert.Equal(t, int64(4096), mem)
}

func TestNodeCapacityFromNode_NilNode(t *testing.T) {
	_, err := NodeCapacityFromNode(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node is nil")
}

func TestNodeCapacityFromNode_InvalidKarpenterCPULabel(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{karpenterInstanceCPULabel: "not-a-number"},
		},
	}
	_, err := NodeCapacityFromNode(node)
	require.Error(t, err)
	assert.Contains(t, err.Error(), karpenterInstanceCPULabel)
}

func TestNodeCapacityFromNode_InvalidKarpenterMemoryLabel(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{karpenterInstanceMemoryLabel: "bad"},
		},
	}
	_, err := NodeCapacityFromNode(node)
	require.Error(t, err)
	assert.Contains(t, err.Error(), karpenterInstanceMemoryLabel)
}

func TestValidateForConfig(t *testing.T) {
	tests := []struct {
		name            string
		capacity        NodeCapacity
		cfg             InjectionConfig
		wantErrContains string
	}{
		{
			name: "cpu-percent set, cpu capacity present",
			capacity: func() NodeCapacity {
				v := int64(2000)
				return NodeCapacity{milliCPU: &v}
			}(),
			cfg: InjectionConfig{CPUPercent: pct(10)},
		},
		{
			name:            "cpu-percent set, cpu capacity missing",
			capacity:        NodeCapacity{},
			cfg:             InjectionConfig{CPUPercent: pct(10)},
			wantErrContains: "cpu-percent is configured but node has no CPU capacity",
		},
		{
			name: "memory-percent set, memory capacity present",
			capacity: func() NodeCapacity {
				v := int64(4096)
				return NodeCapacity{memoryMi: &v}
			}(),
			cfg: InjectionConfig{MemoryPercent: pct(5)},
		},
		{
			name:            "memory-percent set, memory capacity missing",
			capacity:        NodeCapacity{},
			cfg:             InjectionConfig{MemoryPercent: pct(5)},
			wantErrContains: "memory-percent is configured but node has no memory capacity",
		},
		{
			name:     "neither percent set — no validation needed",
			capacity: NodeCapacity{},
			cfg:      InjectionConfig{},
		},
		{
			name: "both set and both present",
			capacity: func() NodeCapacity {
				cpu := int64(4000)
				mem := int64(8192)
				return NodeCapacity{milliCPU: &cpu, memoryMi: &mem}
			}(),
			cfg: InjectionConfig{CPUPercent: pct(10), MemoryPercent: pct(5)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.capacity.ValidateForConfig(tc.cfg)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}
			require.NoError(t, err)
		})
	}
}
