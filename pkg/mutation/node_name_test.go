package mutation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeNameFromPod(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		wantNode    string
		wantErrContains string
	}{
		{
			name: "nodeName set directly",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{NodeName: "worker-42"},
			},
			wantNode: "worker-42",
		},
		{
			name: "node affinity with correct metadata.name match field",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:      metav1.ObjectNameField,
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"worker-99"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantNode: "worker-99",
		},
		{
			name: "affinity with match field that has no values",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:    metav1.ObjectNameField,
												Values: []string{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErrContains: "does not contain a node name match field",
		},
		{
			name: "affinity with no metadata.name match field",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchFields: []corev1.NodeSelectorRequirement{
											{
												Key:    "some-other-field",
												Values: []string{"value"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErrContains: "does not contain a node name match field",
		},
		{
			name: "nil affinity",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{Affinity: nil},
			},
			wantErrContains: "no nodeName or node affinity required during scheduling",
		},
		{
			name: "nil NodeAffinity",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{NodeAffinity: nil},
				},
			},
			wantErrContains: "no nodeName or node affinity required during scheduling",
		},
		{
			name: "nil RequiredDuringScheduling",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: nil,
						},
					},
				},
			},
			wantErrContains: "no required node affinity",
		},
		{
			name: "empty NodeSelectorTerms",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{},
							},
						},
					},
				},
			},
			wantErrContains: "does not contain a node name match field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nodeNameFromPod(tc.pod)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantNode, got)
		})
	}
}
