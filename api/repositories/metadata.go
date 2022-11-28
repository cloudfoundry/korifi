package repositories

import "sigs.k8s.io/controller-runtime/pkg/client"

type Metadata struct {
	Annotations map[string]string
	Labels      map[string]string
}

type MetadataPatch struct {
	Annotations map[string]*string
	Labels      map[string]*string
}

func (p *MetadataPatch) Apply(obj client.Object) {
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}

	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}

	patchMap(obj.GetAnnotations(), p.Annotations)
	patchMap(obj.GetLabels(), p.Labels)
}

func patchMap(original map[string]string, patch map[string]*string) {
	for key, ptrToPatchValue := range patch {
		if ptrToPatchValue != nil {
			original[key] = *ptrToPatchValue
		} else {
			delete(original, key)
		}
	}
}
