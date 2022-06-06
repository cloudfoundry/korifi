package helpers

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MakeCFSpace(namespace string, displayName string) *korifiv1alpha1.CFSpace {
	return &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
			Labels:    map[string]string{korifiv1alpha1.SpaceNameLabel: displayName},
		},
		Spec: korifiv1alpha1.CFSpaceSpec{
			DisplayName: displayName,
		},
	}
}

func MakeCFOrg(cfOrgGUID string, namespace string, name string) *korifiv1alpha1.CFOrg {
	return &korifiv1alpha1.CFOrg{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFOrg",
			APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfOrgGUID,
			Namespace: namespace,
		},
		Spec: korifiv1alpha1.CFOrgSpec{
			DisplayName: name,
		},
	}
}
