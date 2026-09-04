package repositories

import (
	"fmt"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/tools/metadata"
	"code.cloudfoundry.org/korifi/tools"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Metadata struct {
	Annotations map[string]string
	Labels      map[string]string
}

type MetadataPatch struct {
	Annotations map[string]*string
	Labels      map[string]*string
}

func (p *MetadataPatch) Apply(obj client.Object) error {
	if err := p.validate(obj); err != nil {
		return err
	}

	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}

	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}

	patchMap(obj.GetAnnotations(), p.Annotations)
	patchMap(obj.GetLabels(), p.Labels)

	return nil
}

func (p *MetadataPatch) validate(obj client.Object) error {
	for patchKey, patchValue := range p.Annotations {
		if !isUpdate(obj.GetAnnotations(), patchKey, patchValue) {
			continue
		}

		if err := metadata.CloudfoundryKeyCheck(patchKey); err != nil {
			// TODO: the error message probably may not contain that much details, logging them should be sufficient
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid annotations patch: %q: %q -> %q", patchKey, obj.GetAnnotations()[patchKey], tools.ZeroIfNil(patchValue)))
		}
	}

	for patchKey, patchValue := range p.Labels {
		if !isUpdate(obj.GetLabels(), patchKey, patchValue) {
			continue
		}

		if err := metadata.CloudfoundryKeyCheck(patchKey); err != nil {
			return apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("invalid labels patch: %q: %q -> %q", patchKey, obj.GetLabels()[patchKey], tools.ZeroIfNil(patchValue)))
		}
	}

	return nil
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

func isUpdate(valuesMap map[string]string, newKey string, newValuePtr *string) bool {
	if newValuePtr == nil {
		// TODO: I think we do not keep nil metadata values, double check that
		return true
	}

	return valuesMap[newKey] != *newValuePtr
}
