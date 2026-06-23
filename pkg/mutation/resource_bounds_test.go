package mutation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestClampQuantity(t *testing.T) {
	value := resource.MustParse("200m")
	min := resource.MustParse("500m")
	max := resource.MustParse("100m")

	clampedMin := clampQuantity(value, &min, nil)
	assert.Equal(t, "500m", clampedMin.String())

	clampedMax := clampQuantity(value, nil, &max)
	assert.Equal(t, "100m", clampedMax.String())

	mid := resource.MustParse("750m")
	ceiling := resource.MustParse("1")
	clampedMid := clampQuantity(value, &mid, &ceiling)
	assert.Equal(t, "750m", clampedMid.String())
}

func TestParseQuantityLabel(t *testing.T) {
	qty, err := parseQuantityLabel(map[string]string{LabelCPUMin: "100m"}, LabelCPUMin)
	require.NoError(t, err)
	require.NotNil(t, qty)
	assert.Equal(t, "100m", qty.String())

	_, err = parseQuantityLabel(map[string]string{LabelCPUMin: "invalid"}, LabelCPUMin)
	require.Error(t, err)
}

func TestValidateMinMax(t *testing.T) {
	min := resource.MustParse("500m")
	max := resource.MustParse("100m")
	require.Error(t, validateMinMax(&min, &max, "cpu"))
	require.NoError(t, validateMinMax(&max, &min, "cpu"))
}
