package webhooks_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFPlacementValidation", func() {
	var (
		fakeClient         *fake.Client
		placementValidator *webhooks.PlacementValidator
		err                error
		testNamespace      string
		rootNamespace      string

		org   v1alpha1.CFOrg
		space v1alpha1.CFSpace
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		rootNamespace = "cf"
		testNamespace = "foo"

		scheme := runtime.NewScheme()
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		placementValidator = webhooks.NewPlacementValidator(fakeClient, rootNamespace)
	})

	Describe("ValidateOrgCreate", func() {
		BeforeEach(func() {
			org = v1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-org",
					Namespace: testNamespace,
				},
				Spec: v1alpha1.CFOrgSpec{
					DisplayName: "test-org-display-name",
				},
			}
		})

		It("fails if it is not in the root namespace", func() {
			err = placementValidator.ValidateOrgCreate(org)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ValidateSpaceCreate", func() {
		BeforeEach(func() {
			space = v1alpha1.CFSpace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-space",
					Namespace: testNamespace,
				},
				Spec: v1alpha1.CFSpaceSpec{
					DisplayName: "test-space-display-name",
				},
			}
		})

		It("fails if it is not placed in a namespace with a corresponding org", func() {
			fakeClient.GetReturns(errors.New("I don't know what kind of error this normally returns"))
			err = placementValidator.ValidateSpaceCreate(space)
			Expect(err).To(HaveOccurred())
		})
	})
})
