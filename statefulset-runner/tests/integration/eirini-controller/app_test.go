package eirini_controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests/integration"
)

var _ = Describe("App", func() {
	var (
		lrpName    string
		lrpGUID    string
		lrpVersion string
		lrp        *eiriniv1.LRP
		createErr  error
		//serviceName string
	)

	BeforeEach(func() {
		lrpName = tests.GenerateGUID()
		lrpGUID = tests.GenerateGUID()
		lrpVersion = tests.GenerateGUID()
		//serviceName = ""

		lrp = &eiriniv1.LRP{
			ObjectMeta: metav1.ObjectMeta{
				Name: lrpName,
			},
			Spec: eiriniv1.LRPSpec{
				GUID:                   lrpGUID,
				Version:                lrpVersion,
				Image:                  "eirini/dorini",
				AppGUID:                "the-app-guid",
				AppName:                "k-2so",
				SpaceName:              "s",
				OrgName:                "o",
				Env:                    map[string]string{"FOO": "BAR"},
				MemoryMB:               256,
				DiskMB:                 256,
				CPUWeight:              10,
				Instances:              1,
				Ports:                  []int32{8080},
				VolumeMounts:           []eiriniv1.VolumeMount{},
				UserDefinedAnnotations: map[string]string{},
			},
		}
	})

	JustBeforeEach(func() {
		lrp, createErr = fixture.EiriniClientset.
			EiriniV1().
			LRPs(fixture.Namespace).
			Create(context.Background(), lrp, metav1.CreateOptions{})
		//if createErr == nil {
		//	serviceName = tests.ExposeAsService(fixture.Clientset, fixture.Namespace, lrpGUID, 8080, "/")
		//}
	})

	Describe("desiring an app", func() {
		It("create the application", func() {
			Expect(createErr).NotTo(HaveOccurred())

			//	output, err := tests.RequestServiceFn(fixture.Namespace, serviceName, 8080, "/")()
			//	Expect(err).NotTo(HaveOccurred())
			//	Expect(output).To(ContainSubstring("Dora"))
		})

		It("sets the runAsNonRoot in the PodSecurityContext", func() {
			var stsets *v1.StatefulSetList
			Eventually(func(g Gomega) []v1.StatefulSet {
				var err error
				stsets, err = fixture.Clientset.AppsV1().StatefulSets(fixture.Namespace).List(context.Background(), metav1.ListOptions{})
				g.Expect(err).NotTo(HaveOccurred())
				return stsets.Items
			}, "10s").Should(HaveLen(1))

			Expect(stsets.Items[0].Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(PointTo(BeTrue()))
		})

		//When("AllowRunImageAsRoot is true", func() {
		//	BeforeEach(func() {
		//		config.AllowRunImageAsRoot = true
		//	})
		//
		//	It("doesn't set `runAsNonRoot` in the PodSecurityContext", func() {
		//		var stsets *v1.StatefulSetList
		//		Eventually(func(g Gomega) []v1.StatefulSet {
		//			var err error
		//			stsets, err = fixture.Clientset.AppsV1().StatefulSets(fixture.Namespace).List(context.Background(), metav1.ListOptions{})
		//			g.Expect(err).NotTo(HaveOccurred())
		//			return stsets.Items
		//		}, "10s").Should(HaveLen(1))
		//
		//		Expect(stsets.Items[0].Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(BeNil())
		//	})
		//})

		When("DiskMB is zero", func() {
			BeforeEach(func() {
				lrp.Spec.DiskMB = 0
			})

			It("errors", func() {
				Expect(createErr).To(MatchError(ContainSubstring("Invalid value")))
			})
		})

		//Describe("automounting serviceacccount token", func() {
		//	const serviceAccountTokenMountPath = "/var/run/secrets/kubernetes.io/serviceaccount"
		//
		//	It("does not mount the service account token", func() {
		//		result, err := tests.RequestServiceFn(fixture.Namespace, serviceName, 8080, fmt.Sprintf("/ls?path=%s", serviceAccountTokenMountPath))()
		//		Expect(err).To(MatchError(ContainSubstring("Internal Server Error")))
		//		Expect(result).To(ContainSubstring("no such file or directory"))
		//	})
		//
		//	When("unsafe_allow_automount_service_account_token is set", func() {
		//		BeforeEach(func() {
		//			config.UnsafeAllowAutomountServiceAccountToken = true
		//		})
		//
		//		It("mounts the service account token (because this is how K8S works by default)", func() {
		//			_, err := tests.RequestServiceFn(fixture.Namespace, serviceName, 8080, fmt.Sprintf("/ls?path=%s", serviceAccountTokenMountPath))()
		//			Expect(err).NotTo(HaveOccurred())
		//		})
		//
		//		When("the app service account has its automountServiceAccountToken set to false", func() {
		//			updateServiceaccount := func() error {
		//				appServiceAccount, err := fixture.Clientset.CoreV1().ServiceAccounts(fixture.Namespace).Get(context.Background(), tests.GetApplicationServiceAccount(), metav1.GetOptions{})
		//				Expect(err).NotTo(HaveOccurred())
		//				automountServiceAccountToken := false
		//				appServiceAccount.AutomountServiceAccountToken = &automountServiceAccountToken
		//				_, err = fixture.Clientset.CoreV1().ServiceAccounts(fixture.Namespace).Update(context.Background(), appServiceAccount, metav1.UpdateOptions{})
		//
		//				return err
		//			}
		//
		//			BeforeEach(func() {
		//				Eventually(updateServiceaccount, "5s").Should(Succeed())
		//			})
		//
		//			It("does not mount the service account token", func() {
		//				result, err := tests.RequestServiceFn(fixture.Namespace, serviceName, 8080, fmt.Sprintf("/ls?path=%s", serviceAccountTokenMountPath))()
		//				Expect(err).To(MatchError(ContainSubstring("Internal Server Error")))
		//				Expect(result).To(ContainSubstring("no such file or directory"))
		//			})
		//		})
		//	})
		//})
	})

	Describe("Update an app", func() {
		var updatedLRP *eiriniv1.LRP

		BeforeEach(func() {
			updatedLRP = lrp.DeepCopy()
			updatedLRP.Spec.Instances = 3
		})

		JustBeforeEach(func() {
			Expect(createErr).NotTo(HaveOccurred())
			updatedLRP.ResourceVersion = integration.GetLRP(fixture.EiriniClientset, fixture.Namespace, lrpName).ResourceVersion

			_, err := fixture.EiriniClientset.
				EiriniV1().
				LRPs(fixture.Namespace).
				Update(context.Background(), updatedLRP, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("updates the underlying statefulset", func() {
			Eventually(func() int32 {
				return *integration.GetStatefulSet(fixture.Clientset, fixture.Namespace, lrpGUID, lrpVersion).Spec.Replicas
			}).Should(Equal(int32(3)))
		})

		//It("updates the LRP custom resource status", func() {
		//	Eventually(func() int32 {
		//		return integration.GetLRP(fixture.EiriniClientset, fixture.Namespace, lrpName).Status.Replicas
		//	}).Should(Equal(int32(3)))
		//})
	})

	//Describe("Stop an app", func() {
	//	JustBeforeEach(func() {
	//		Expect(fixture.EiriniClientset.
	//			EiriniV1().
	//			LRPs(fixture.Namespace).
	//			Delete(context.Background(), lrpName, metav1.DeleteOptions{}),
	//		).To(Succeed())
	//	})
	//
	//	It("stops the application", func() {
	//		Eventually(func() error {
	//			_, err := tests.RequestServiceFn(fixture.Namespace, serviceName, 8080, "/")()
	//
	//			return err
	//		}).Should(MatchError(ContainSubstring("context deadline exceeded")))
	//	})
	//})
	//
	//Describe("App status", func() {
	//	getLRPReplicas := func() int {
	//		l, err := fixture.EiriniClientset.
	//			EiriniV1().
	//			LRPs(fixture.Namespace).
	//			Get(context.Background(), lrpName, metav1.GetOptions{})
	//
	//		Expect(err).NotTo(HaveOccurred())
	//
	//		return int(l.Status.Replicas)
	//	}
	//
	//	When("an app instance becomes unready", func() {
	//		JustBeforeEach(func() {
	//			appListOpts := metav1.ListOptions{
	//				LabelSelector: fmt.Sprintf("%s=%s,%s=%s", stset.LabelGUID, lrpGUID, stset.LabelVersion, lrpVersion),
	//			}
	//			Expect(fixture.Clientset.
	//				CoreV1().
	//				Pods(fixture.Namespace).
	//				DeleteCollection(context.Background(), metav1.DeleteOptions{}, appListOpts),
	//			).To(Succeed())
	//		})
	//
	//		It("is reflected in the LRP status replicas", func() {
	//			Eventually(getLRPReplicas).Should(Equal(0))
	//			Eventually(getLRPReplicas).Should(Equal(1))
	//		})
	//	})
	//})
})
