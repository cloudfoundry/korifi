package validation_test

import (
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFPlacementValidation", func() {
	var (
		fakeClient         *fake.Client
		placementValidator *validation.PlacementValidator
		validationErr      error
		rootNamespace      string

		space korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		fakeClient = new(fake.Client)

		rootNamespace = "cf"

		scheme := runtime.NewScheme()
		Expect(korifiv1alpha1.AddToScheme(scheme)).To(Succeed())

		placementValidator = validation.NewPlacementValidator(fakeClient, rootNamespace)
	})

	Describe("ValidateOrgCreate", func() {
		var org korifiv1alpha1.CFOrg

		BeforeEach(func() {
			org = korifiv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-org",
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFOrgSpec{
					DisplayName: "test-org-display-name",
				},
			}
		})

		JustBeforeEach(func() {
			validationErr = placementValidator.ValidateOrgCreate(org)
		})

		It("succeeds", func() {
			Expect(validationErr).NotTo(HaveOccurred())
		})

		When("the org is not in the root namespace", func() {
			BeforeEach(func() {
				org.ObjectMeta.Namespace = "foo"
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					validation.OrgPlacementErrorType,
					Equal(fmt.Sprintf(validation.OrgPlacementErrorMessage, org.Spec.DisplayName)),
				))
			})
		})
	})

	Describe("ValidateSpaceCreate", func() {
		BeforeEach(func() {
			space = korifiv1alpha1.CFSpace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-space",
					Namespace: "org-ns",
				},
				Spec: korifiv1alpha1.CFSpaceSpec{
					DisplayName: "test-space-display-name",
				},
			}
		})

		JustBeforeEach(func() {
			validationErr = placementValidator.ValidateSpaceCreate(space)
		})

		It("succeeds", func() {
			Expect(validationErr).NotTo(HaveOccurred())
		})

		When("the space is not placed in an org namespace", func() {
			BeforeEach(func() {
				fakeClient.GetReturns(errors.New("I don't know what kind of error this normally returns"))
			})

			It("fails", func() {
				Expect(validationErr).To(matchers.BeValidationError(
					validation.SpacePlacementErrorType,
					Equal(fmt.Sprintf(validation.SpacePlacementErrorMessage, "org-ns", space.Spec.DisplayName)),
				))
			})
		})
	})
})
