package v1alpha1_test

import (
	"strconv"

	"code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFAppMutatingWebhook Unit Tests", func() {
	const (
		cfAppGUID        = "test-app-guid"
		cfAppLabelKey    = "workloads.cloudfoundry.org/app-guid"
		cfAppRevisionKey = "workloads.cloudfoundry.org/app-rev"
		namespace        = "default"
	)

	When("there are no existing labels on the CFAPP record", func() {
		It("should add a new label matching metadata.name", func() {
			cfApp := &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFAppSpec{
					DisplayName:  "test-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}

			cfApp.Default()
			Expect(cfApp.ObjectMeta.Labels).To(HaveKeyWithValue(cfAppLabelKey, cfAppGUID))
			Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "0"))
		})
	})

	When("there are other existing labels on the CFAPP record", func() {
		It("should add a new label matching metadata.name and preserve the other labels", func() {
			cfApp := &v1alpha1.CFApp{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFApp",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfAppGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "app-label",
					},
					Annotations: map[string]string{
						"someAnnotation": "blah",
					},
				},
				Spec: v1alpha1.CFAppSpec{
					DisplayName:  "test-app",
					DesiredState: "STOPPED",
					Lifecycle: v1alpha1.Lifecycle{
						Type: "buildpack",
					},
				},
			}

			cfApp.Default()
			Expect(cfApp.ObjectMeta.Labels).To(HaveLen(2))
			Expect(cfApp.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "app-label"))
			Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, "0"))
		})
	})

	When("the app desiredState STARTED->STOPPED and status.observedDesiredState is STARTED and", func() {
		When("rev is set to an integer value", func() {
			const (
				revisionValue = 7
			)
			var cfApp *v1alpha1.CFApp
			BeforeEach(func() {
				cfApp = initializeCFAppCR(cfAppGUID, namespace)
				cfApp.Spec.DesiredState = v1alpha1.StoppedState
				cfApp.Annotations[v1alpha1.CFAppRevisionKey] = strconv.Itoa(revisionValue)
				cfApp.Status.ObservedDesiredState = v1alpha1.StartedState
			})

			It("should increment the rev", func() {
				cfApp.Default()
				Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, strconv.Itoa(revisionValue+1)))
			})
		})

		When("rev is set to some non-integer value", func() {
			var cfApp *v1alpha1.CFApp
			BeforeEach(func() {
				cfApp = initializeCFAppCR(cfAppGUID, namespace)
				cfApp.Spec.DesiredState = v1alpha1.StoppedState
				cfApp.Annotations[v1alpha1.CFAppRevisionKey] = "some-weird-value"
				cfApp.Status.ObservedDesiredState = v1alpha1.StartedState
			})

			It("should set the rev to be the default value", func() {
				cfApp.Default()
				Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, v1alpha1.CFAppRevisionKeyDefault))
			})
		})
	})

	When("the app desiredState STOPPED->STARTED and status.observedDesiredState is STOPPED and", func() {
		When("rev is set to an integer value", func() {
			const (
				revisionValue = 7
			)
			var cfApp *v1alpha1.CFApp
			BeforeEach(func() {
				cfApp = initializeCFAppCR(cfAppGUID, namespace)
				cfApp.Spec.DesiredState = v1alpha1.StartedState
				cfApp.Annotations[v1alpha1.CFAppRevisionKey] = strconv.Itoa(revisionValue)
				cfApp.Status.ObservedDesiredState = v1alpha1.StoppedState
			})

			It("should leave the rev alone", func() {
				cfApp.Default()
				Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, strconv.Itoa(revisionValue)))
			})
		})

		When("rev is set to some non-integer value", func() {
			const (
				weirdRevValue = "some-weird-value"
			)
			var cfApp *v1alpha1.CFApp
			BeforeEach(func() {
				cfApp = initializeCFAppCR(cfAppGUID, namespace)
				cfApp.Spec.DesiredState = v1alpha1.StartedState
				cfApp.Annotations[v1alpha1.CFAppRevisionKey] = weirdRevValue
				cfApp.Status.ObservedDesiredState = v1alpha1.StoppedState
			})

			It("should leave the rev alone", func() {
				cfApp.Default()
				Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, weirdRevValue))
			})
		})

		When("rev is not set", func() {
			var cfApp *v1alpha1.CFApp
			BeforeEach(func() {
				cfApp = initializeCFAppCR(cfAppGUID, namespace)
				cfApp.Spec.DesiredState = v1alpha1.StartedState
				cfApp.Status.ObservedDesiredState = v1alpha1.StoppedState
			})

			It("should set it to the default value", func() {
				cfApp.Default()
				Expect(cfApp.ObjectMeta.Annotations).To(HaveKeyWithValue(cfAppRevisionKey, v1alpha1.CFAppRevisionKeyDefault))
			})
		})
	})
})

func initializeCFAppCR(appGUID, namespace string) *v1alpha1.CFApp {
	return &v1alpha1.CFApp{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFApp",
			APIVersion: v1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        appGUID,
			Namespace:   namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1alpha1.CFAppSpec{
			DisplayName:  "test-app",
			DesiredState: "STOPPED",
			Lifecycle: v1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
}
