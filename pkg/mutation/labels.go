package mutation

// Label keys used by kube-tailor mutations.
// Apply DaemonSet resource labels to + metadata (pod template labels as fallback).
const (
	LabelPrefix        = "kube-tailor/"
	LabelEnabled       = LabelPrefix + "enabled"
	LabelCPUPercent    = LabelPrefix + "cpu-percent"
	LabelCPUMin        = LabelPrefix + "cpu-min"
	LabelCPUMax        = LabelPrefix + "cpu-max"
	LabelMemoryPercent = LabelPrefix + "memory-percent"
	LabelMemoryMin     = LabelPrefix + "memory-min"
	LabelMemoryMax     = LabelPrefix + "memory-max"
	LabelTargetedContainers = LabelPrefix + "targetted-containers"
)
