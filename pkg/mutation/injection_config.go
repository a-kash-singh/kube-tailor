package mutation

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// InjectionConfig holds per-DaemonSet resource injection settings parsed from labels.
type InjectionConfig struct {
	Enabled       bool
	CPUPercent    *float64
	CPUMin        *resource.Quantity
	CPUMax        *resource.Quantity
	MemoryPercent *float64
	MemoryMin     *resource.Quantity
	MemoryMax     *resource.Quantity
	TargetedContainers []string
}

// ParseInjectionConfig reads injection settings from label maps.
// When enabled is true, at least one of cpu-percent or memory-percent must be set.
func ParseInjectionConfig(labels map[string]string) (InjectionConfig, error) {
	cfg := InjectionConfig{}

	if labels == nil {
		return cfg, nil
	}

	enabled, err := parseBoolLabel(labels, LabelEnabled)
	if err != nil {
		return cfg, err
	}
	cfg.Enabled = enabled
	if !cfg.Enabled {
		return cfg, nil
	}

	cpuPercent, cpuSet, err := parsePercentLabel(labels, LabelCPUPercent)
	if err != nil {
		return cfg, err
	}
	if cpuSet {
		cfg.CPUPercent = &cpuPercent
	}

	cfg.CPUMin, err = parseQuantityLabel(labels, LabelCPUMin)
	if err != nil {
		return cfg, err
	}
	cfg.CPUMax, err = parseQuantityLabel(labels, LabelCPUMax)
	if err != nil {
		return cfg, err
	}

	memPercent, memSet, err := parsePercentLabel(labels, LabelMemoryPercent)
	if err != nil {
		return cfg, err
	}
	if memSet {
		cfg.MemoryPercent = &memPercent
	}

	cfg.MemoryMin, err = parseQuantityLabel(labels, LabelMemoryMin)
	if err != nil {
		return cfg, err
	}
	cfg.MemoryMax, err = parseQuantityLabel(labels, LabelMemoryMax)
	if err != nil {
		return cfg, err
	}

	cfg.TargetedContainers = parseCSVStringsLabel(labels, LabelTargetedContainers)

	if !cpuSet && !memSet {
		return cfg, fmt.Errorf(
			"daemonset has %q but requires at least one of %q or %q",
			LabelEnabled, LabelCPUPercent, LabelMemoryPercent,
		)
	}

	if cfg.CPUMin != nil || cfg.CPUMax != nil {
		if !cpuSet {
			return cfg, fmt.Errorf("%q and %q require %q", LabelCPUMin, LabelCPUMax, LabelCPUPercent)
		}
	}

	if cfg.MemoryMin != nil || cfg.MemoryMax != nil {
		if !memSet {
			return cfg, fmt.Errorf("%q and %q require %q", LabelMemoryMin, LabelMemoryMax, LabelMemoryPercent)
		}
	}

	return cfg, nil
}

// MergeLabelMaps returns primary labels with missing keys filled from fallback.
func MergeLabelMaps(primary, fallback map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range fallback {
		merged[k] = v
	}
	for k, v := range primary {
		merged[k] = v
	}
	return merged
}

func parseBoolLabel(labels map[string]string, key string) (bool, error) {
	value, ok := labels[key]
	if !ok {
		return false, nil
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("label %q must be a boolean, got %q", key, value)
	}
}

func parsePercentLabel(labels map[string]string, key string) (float64, bool, error) {
	value, ok := labels[key]
	if !ok {
		return 0, false, nil
	}

	percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, true, fmt.Errorf("label %q must be a number, got %q: %w", key, value, err)
	}
	if percent <= 0 || percent > 100 {
		return 0, true, fmt.Errorf("label %q must be between 0 and 100 exclusive, got %g", key, percent)
	}

	return percent, true, nil
}

func parseCSVStringsLabel(labels map[string]string, key string) []string {
	if labels == nil {
		return nil
	}

	raw, ok := labels[key]
	if !ok {
		return nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
