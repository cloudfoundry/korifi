package k8s

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

func MatchNotingSelector() labels.Selector {
	r1, _ := labels.NewRequirement("whatever", selection.Exists, []string{})
	r2, _ := labels.NewRequirement("whatever", selection.DoesNotExist, []string{})

	return labels.NewSelector().Add(*r1, *r2)
}
