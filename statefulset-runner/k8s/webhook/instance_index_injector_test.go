package webhook_test

import (
	"context"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/webhook"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("InstanceIndexInjector", func() {
	var (
		injector *webhook.InstanceIndexEnvInjector
		logger   lager.Logger
		pod      *corev1.Pod
		req      admission.Request
		resp     admission.Response
	)

	BeforeEach(func() {
		logger = tests.NewTestLogger("instance-index-injector")
		decoder, err := admission.NewDecoder(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		injector = webhook.NewInstanceIndexEnvInjector(logger, decoder)

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-app-instance-3",
				Labels: map[string]string{
					stset.LabelSourceType: stset.AppSourceType,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "opi",
						Env: []corev1.EnvVar{
							{Name: "FOO", Value: "foo"},
							{Name: "BAR", Value: "bar"},
						},
					},
				},
			},
		}

		req = admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				Object:    rawExt(pod),
			},
		}
	})

	JustBeforeEach(func() {
		resp = injector.Handle(context.Background(), req)
	})

	It("injects the app instance as env variable in the container", func() {
		Expect(resp.Patches).To(Equal(
			[]jsonpatch.Operation{
				{
					Operation: "add",
					Path:      "/spec/containers/0/env/2",
					Value: map[string]interface{}{
						"name":  "CF_INSTANCE_INDEX",
						"value": "3",
					},
				},
			},
		))
	})

	Context("the passed pod has already been created", func() {
		When("operation is Update", func() {
			BeforeEach(func() {
				req.AdmissionRequest.Operation = admissionv1.Update
			})

			It("allows the operation without interacting with the passed pod", func() {
				ExpectAllowResponse(resp, "pod was already created")
			})
		})

		When("operation is Delete", func() {
			BeforeEach(func() {
				req.AdmissionRequest.Operation = admissionv1.Delete
			})

			It("allows the operation without interacting with the passed pod", func() {
				ExpectAllowResponse(resp, "pod was already created")
			})
		})

		When("operation is Connect", func() {
			BeforeEach(func() {
				req.AdmissionRequest.Operation = admissionv1.Connect
			})

			It("allows the operation without interacting with the passed pod", func() {
				ExpectAllowResponse(resp, "pod was already created")
			})
		})
	})

	When("the pod name has no dashes", func() {
		BeforeEach(func() {
			pod.Name = "myinstance4"
			req.Object = rawExt(pod)
		})

		It("returns an error response", func() {
			ExpectBadRequestErrorResponse(resp, "could not parse app name")
		})
	})

	When("the pod name is empty", func() {
		BeforeEach(func() {
			pod.Name = ""
			req.Object = rawExt(pod)
		})

		It("returns an error response", func() {
			ExpectBadRequestErrorResponse(resp, "could not parse app name")
		})
	})

	When("pod name part after final dash is not numeric", func() {
		BeforeEach(func() {
			pod.Name = "my-instance-four"
			req.Object = rawExt(pod)
		})

		It("returns an error response", func() {
			ExpectBadRequestErrorResponse(resp, "pod my-instance-four name does not contain an index")
		})
	})

	When("pod name ends with a dash", func() {
		BeforeEach(func() {
			pod.Name = "my-instance-"
			req.Object = rawExt(pod)
		})

		It("returns an error response", func() {
			ExpectBadRequestErrorResponse(resp, "pod my-instance- name does not contain an index")
		})
	})

	When("the pod has no app container", func() {
		BeforeEach(func() {
			pod.Spec.Containers[0].Name = "ipo"
			req.Object = rawExt(pod)
		})

		It("returns an error response", func() {
			ExpectBadRequestErrorResponse(resp, "no application container found in pod")
		})
	})
})

func ExpectBadRequestErrorResponse(resp admission.Response, msg string) {
	ExpectWithOffset(1, resp.Allowed).To(BeFalse())
	ExpectWithOffset(1, resp.Result).ToNot(BeNil())
	ExpectWithOffset(1, resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
	ExpectWithOffset(1, resp.Result.Message).To(ContainSubstring(msg))
	ExpectWithOffset(1, resp.Patches).To(BeEmpty())
}

func ExpectAllowResponse(resp admission.Response, reason string) {
	ExpectWithOffset(1, resp.Allowed).To(BeTrue())
	ExpectWithOffset(1, resp.Result).ToNot(BeNil())
	ExpectWithOffset(1, resp.Result.Code).To(Equal(int32(http.StatusOK)))
	ExpectWithOffset(1, resp.Result.Reason).To(Equal(metav1.StatusReason(reason)))
	ExpectWithOffset(1, resp.Patches).To(BeEmpty())
}

func rawExt(obj interface{}) runtime.RawExtension {
	rawObj, err := json.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())

	return runtime.RawExtension{Raw: rawObj}
}
