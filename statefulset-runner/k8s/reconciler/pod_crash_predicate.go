package reconciler

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type SourceTypeUpdatePredicate struct {
	sourceType string
}

func NewSourceTypeUpdatePredicate(sourceType string) SourceTypeUpdatePredicate {
	return SourceTypeUpdatePredicate{sourceType: sourceType}
}

func (p SourceTypeUpdatePredicate) Update(e event.UpdateEvent) bool {
	return e.ObjectNew.GetLabels()[stset.LabelSourceType] == p.sourceType
}

func (SourceTypeUpdatePredicate) Create(event.CreateEvent) bool {
	return false
}

func (SourceTypeUpdatePredicate) Delete(event.DeleteEvent) bool {
	return false
}

func (SourceTypeUpdatePredicate) Generic(event.GenericEvent) bool {
	return false
}
