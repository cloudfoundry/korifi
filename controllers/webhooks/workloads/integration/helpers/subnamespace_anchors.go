package helpers

import (
	"code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

func MakeOrg(namespace, name string) *hnsv1alpha2.SubnamespaceAnchor {
	return MakeSubnamespaceAnchor(namespace, map[string]string{workloads.OrgNameLabel: name})
}

func MakeSpace(namespace, name string) *hnsv1alpha2.SubnamespaceAnchor {
	return MakeSubnamespaceAnchor(namespace, map[string]string{workloads.SpaceNameLabel: name})
}

func MakeCFSpace(namespace string, displayName string) *workloadsv1alpha1.CFSpace {
	return &workloadsv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
			Labels:    map[string]string{workloads.SpaceNameLabel: displayName},
		},
		Spec: workloadsv1alpha1.CFSpaceSpec{
			DisplayName: displayName,
		},
	}
}

func MakeCFOrg(cfOrgGUID string, namespace string, name string) *v1alpha1.CFOrg {
	return &v1alpha1.CFOrg{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFOrg",
			APIVersion: v1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfOrgGUID,
			Namespace: namespace,
		},
		Spec: v1alpha1.CFOrgSpec{
			DisplayName: name,
		},
	}
}

func MakeSubnamespaceAnchor(namespace string, labels map[string]string) *hnsv1alpha2.SubnamespaceAnchor {
	return &hnsv1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: namespace,
			Labels:    labels,
		},
	}
}
