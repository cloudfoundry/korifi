package testutils

import (
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func InitializeCFRoute(cfRouteGUID, namespace, cfDomainGUID, cfRouteHost string) *networkingv1alpha1.CFRoute {
	return &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfRouteGUID,
			Namespace: namespace,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host: cfRouteHost,
			DomainRef: v1.LocalObjectReference{
				Name: cfDomainGUID,
			},
		},
	}
}

func InitializeCFDomain(cfDomainGUID, cfDomainName string) *networkingv1alpha1.CFDomain {
	return &networkingv1alpha1.CFDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfDomainGUID,
		},
		Spec: networkingv1alpha1.CFDomainSpec{
			Name: cfDomainName,
		},
	}
}
