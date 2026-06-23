package mutation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseInjectionConfigDisabled(t *testing.T) {
	cfg, err := ParseInjectionConfig(nil)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
}

func TestParseInjectionConfigEnabledCPUOnly(t *testing.T) {
	cfg, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:    "true",
		LabelCPUPercent: "10",
	})
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.NotNil(t, cfg.CPUPercent)
	assert.Equal(t, 10.0, *cfg.CPUPercent)
	assert.Nil(t, cfg.MemoryPercent)
}

func TestParseInjectionConfigEnabledMemoryOnly(t *testing.T) {
	cfg, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:       "true",
		LabelMemoryPercent: "5",
	})
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	assert.Nil(t, cfg.CPUPercent)
	require.NotNil(t, cfg.MemoryPercent)
	assert.Equal(t, 5.0, *cfg.MemoryPercent)
}

func TestParseInjectionConfigEnabledBoth(t *testing.T) {
	cfg, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:       "true",
		LabelCPUPercent:    "10",
		LabelMemoryPercent: "5",
	})
	require.NoError(t, err)
	require.NotNil(t, cfg.CPUPercent)
	require.NotNil(t, cfg.MemoryPercent)
	assert.Equal(t, 10.0, *cfg.CPUPercent)
	assert.Equal(t, 5.0, *cfg.MemoryPercent)
}

func TestParseInjectionConfigRequiresPercentWhenEnabled(t *testing.T) {
	_, err := ParseInjectionConfig(map[string]string{
		LabelEnabled: "true",
	})
	require.Error(t, err)
}

func TestParseInjectionConfigRejectsInvalidPercent(t *testing.T) {
	_, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:    "true",
		LabelCPUPercent: "101",
	})
	require.Error(t, err)
}

func TestParseInjectionConfigCPUMinRequiresPercent(t *testing.T) {
	_, err := ParseInjectionConfig(map[string]string{
		LabelEnabled: "true",
		LabelCPUMin:  "100m",
		LabelMemoryPercent: "5",
	})
	require.Error(t, err)
}

func TestParseInjectionConfigAllowsCPUMinGreaterThanMax(t *testing.T) {
	_, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:    "true",
		LabelCPUPercent: "10",
		LabelCPUMin:     "500m",
		LabelCPUMax:     "100m",
	})
	require.NoError(t, err)
}

func TestParseInjectionConfigParsesMinMax(t *testing.T) {
	cfg, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:       "true",
		LabelCPUPercent:    "10",
		LabelCPUMin:        "500m",
		LabelCPUMax:        "2",
		LabelMemoryPercent: "5",
		LabelMemoryMin:     "512Mi",
		LabelMemoryMax:     "2Gi",
	})
	require.NoError(t, err)
	require.NotNil(t, cfg.CPUMin)
	require.NotNil(t, cfg.CPUMax)
	require.NotNil(t, cfg.MemoryMin)
	require.NotNil(t, cfg.MemoryMax)
	assert.Equal(t, "500m", cfg.CPUMin.String())
	assert.Equal(t, "2", cfg.CPUMax.String())
	assert.Equal(t, "512Mi", cfg.MemoryMin.String())
	assert.Equal(t, "2Gi", cfg.MemoryMax.String())
}

func TestNodeCapacityFromKarpenterLabels(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker-1",
			Labels: map[string]string{
				karpenterInstanceCPULabel:    "2",
				karpenterInstanceMemoryLabel: "8192",
			},
		},
	}

	capacity, err := NodeCapacityFromNode(node)
	require.NoError(t, err)

	milliCPU, err := capacity.MilliCPUValue()
	require.NoError(t, err)
	assert.Equal(t, int64(2000), milliCPU)

	memoryMi, err := capacity.MemoryMiValue()
	require.NoError(t, err)
	assert.Equal(t, int64(8192), memoryMi)
}

func TestNodeCapacityFromStatusCapacity(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8192Mi"),
			},
		},
	}

	capacity, err := NodeCapacityFromNode(node)
	require.NoError(t, err)

	milliCPU, err := capacity.MilliCPUValue()
	require.NoError(t, err)
	assert.Equal(t, int64(2000), milliCPU)

	memoryMi, err := capacity.MemoryMiValue()
	require.NoError(t, err)
	assert.Equal(t, int64(8192), memoryMi)
}

func TestCPUQuantityFromPercent(t *testing.T) {
	cpu := cpuQuantityFromPercent(2000, 10)
	assert.Equal(t, "200m", cpu.String())
}

func TestMemoryQuantityFromPercent(t *testing.T) {
	memory := memoryQuantityFromPercent(8192, 5)
	assert.Equal(t, "409Mi", memory.String())
}

func TestParseInjectionConfigTargetsContainers(t *testing.T) {
	cfg, err := ParseInjectionConfig(map[string]string{
		LabelEnabled:            "true",
		LabelCPUPercent:         "10",
		LabelTargetedContainers: "abc, xyz",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"abc", "xyz"}, cfg.TargetedContainers)
}
