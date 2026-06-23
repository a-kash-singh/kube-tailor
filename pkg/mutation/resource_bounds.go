package mutation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

func parseQuantityLabel(labels map[string]string, key string) (*resource.Quantity, error) {
	value, ok := labels[key]
	if !ok {
		return nil, nil
	}

	qty, err := resource.ParseQuantity(value)
	if err != nil {
		return nil, fmt.Errorf("label %q must be a valid quantity, got %q: %w", key, value, err)
	}
	if qty.Sign() <= 0 {
		return nil, fmt.Errorf("label %q must be greater than zero, got %q", key, value)
	}

	parsed := qty.DeepCopy()
	return &parsed, nil
}

func validateMinMax(min, max *resource.Quantity, resourceName string) error {
	if min != nil && max != nil && min.Cmp(*max) > 0 {
		return fmt.Errorf("%s: min must be less than or equal to max", resourceName)
	}
	return nil
}

func clampQuantity(value resource.Quantity, min, max *resource.Quantity) resource.Quantity {
	clamped := value.DeepCopy()
	if min != nil && clamped.Cmp(*min) < 0 {
		clamped = min.DeepCopy()
	}
	if max != nil && clamped.Cmp(*max) > 0 {
		clamped = max.DeepCopy()
	}
	return clamped
}
