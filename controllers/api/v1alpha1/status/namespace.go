package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NamespaceStatus interface {
	GetConditions() *[]metav1.Condition
	SetGUID(string)
	SetObservedGeneration(int64)
}
