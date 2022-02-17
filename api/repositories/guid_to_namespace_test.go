package repositories_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GuidToNamespace", func() {
	var (
		guidToNamespace     repositories.GUIDToNamespace
		ctx                 context.Context
		namespace           *corev1.Namespace
		serviceInstanceGUID string
		nsResult            string
		getNSErr            error
	)

	createServiceInstance := func(ns, name string) {
		Expect(k8sClient.Create(ctx, &servicesv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec: servicesv1alpha1.CFServiceInstanceSpec{
				Type: "user-provided",
			},
		})).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()
		guidToNamespace = repositories.NewGUIDToNamespace(k8sClient)
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: prefixedGUID("namespace"),
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		serviceInstanceGUID = prefixedGUID("service-instance")
		createServiceInstance(namespace.Name, serviceInstanceGUID)
	})

	JustBeforeEach(func() {
		nsResult, getNSErr = guidToNamespace.GetNamespaceForServiceInstance(ctx, serviceInstanceGUID)
	})

	It("returns the namespace for a unique GUID", func() {
		Expect(getNSErr).NotTo(HaveOccurred())
		Expect(nsResult).To(Equal(namespace.Name))
	})

	When("the guid does not exist", func() {
		BeforeEach(func() {
			serviceInstanceGUID = "does-not-exist"
		})

		It("returns a not found error", func() {
			Expect(getNSErr).To(MatchError(repositories.NewNotFoundError(repositories.ServiceInstanceResourceType, nil)))
		})
	})

	When("there are duplicate guids", func() {
		BeforeEach(func() {
			namespace2 := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: prefixedGUID("namespace2")}}
			Expect(k8sClient.Create(ctx, &namespace2)).To(Succeed())
			createServiceInstance(namespace2.Name, serviceInstanceGUID)
		})

		It("returns a duplicate error", func() {
			Expect(getNSErr).To(MatchError(ContainSubstring("duplicate")))
		})
	})
})
