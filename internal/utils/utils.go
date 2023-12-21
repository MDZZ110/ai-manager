package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func ReplaceModelName(model string) string {
	return strings.Replace(model, "/", "-", -1)
}

func ContainsFinalizer(finalizers []string, finalizer string) bool {
	for _, s := range finalizers {
		if s == finalizer {
			return true
		}
	}

	return false
}

func MergeOwnerReferences(old []metav1.OwnerReference, new []metav1.OwnerReference) []metav1.OwnerReference {
	existing := make(map[metav1.OwnerReference]bool)
	for _, ownerRef := range old {
		existing[ownerRef] = true
	}
	for _, ownerRef := range new {
		if _, ok := existing[ownerRef]; !ok {
			old = append(old, ownerRef)
		}
	}
	return old
}

// WithLabels aggregates existing labels
func WithLabels(labels map[string]string, existing map[string]string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}

	for k, v := range existing {
		_, ok := labels[k]
		if !ok {
			labels[k] = v
		}
	}

	return labels
}
