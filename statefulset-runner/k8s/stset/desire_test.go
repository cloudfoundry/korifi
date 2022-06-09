package stset_test

import (
	"context"
	"encoding/base64"
	"fmt"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset/stsetfakes"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Desirer", func() {
	var (
		logger                     lager.Logger
		client                     *k8sfakes.FakeClient
		lrpToStatefulSetConverter  *stsetfakes.FakeLRPToStatefulSetConverter
		podDisruptionBudgetUpdater *stsetfakes.FakePodDisruptionBudgetUpdater
		lrp                        *eiriniv1.LRP
		desirer                    *stset.Desirer
		desireErr                  error
	)

	BeforeEach(func() {
		logger = tests.NewTestLogger("statefulset-desirer")
		client = new(k8sfakes.FakeClient)
		lrpToStatefulSetConverter = new(stsetfakes.FakeLRPToStatefulSetConverter)
		lrpToStatefulSetConverter.ConvertStub = func(statefulSetName string, lrp *eiriniv1.LRP, _ *corev1.Secret) (*appsv1.StatefulSet, error) {
			return &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: statefulSetName,
				},
			}, nil
		}

		podDisruptionBudgetUpdater = new(stsetfakes.FakePodDisruptionBudgetUpdater)
		lrp = createLRP("the-namespace", "Baldur")
		desirer = stset.NewDesirer(logger, lrpToStatefulSetConverter, podDisruptionBudgetUpdater, client, eirinischeme.Scheme)
	})

	JustBeforeEach(func() {
		desireErr = desirer.Desire(ctx, lrp)
	})

	It("succeeds", func() {
		Expect(desireErr).NotTo(HaveOccurred())
	})

	It("creates the StatefulSet", func() {
		Expect(client.CreateCallCount()).To(Equal(1))
		_, obj, _ := client.CreateArgsForCall(0)
		Expect(obj).To(BeAssignableToTypeOf(&appsv1.StatefulSet{}))
		statefulSet := obj.(*appsv1.StatefulSet)
		Expect(statefulSet.Name).To(Equal("baldur-space-foo-34f869d015"))
		Expect(statefulSet.Namespace).To(Equal("the-namespace"))
	})

	It("updates the pod disruption budget", func() {
		Expect(podDisruptionBudgetUpdater.UpdateCallCount()).To(Equal(1))
		_, actualStatefulSet, actualLRP := podDisruptionBudgetUpdater.UpdateArgsForCall(0)
		Expect(actualStatefulSet.Namespace).To(Equal("the-namespace"))
		Expect(actualStatefulSet.Name).To(Equal("baldur-space-foo-34f869d015"))
		Expect(actualLRP).To(Equal(lrp))
	})

	When("updating the pod disruption budget fails", func() {
		BeforeEach(func() {
			podDisruptionBudgetUpdater.UpdateReturns(errors.New("update-error"))
		})

		It("returns an error", func() {
			Expect(desireErr).To(MatchError(ContainSubstring("update-error")))
		})
	})

	When("the app name contains unsupported characters", func() {
		BeforeEach(func() {
			lrp = createLRP("the-namespace", "Балдър")
		})

		It("should use the guid as a name", func() {
			Expect(client.CreateCallCount()).To(Equal(1))
			_, obj, _ := client.CreateArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&appsv1.StatefulSet{}))
			statefulSet := obj.(*appsv1.StatefulSet)
			Expect(statefulSet.Name).To(Equal("guid_1234-34f869d015"))
		})
	})

	When("the statefulset already exists", func() {
		BeforeEach(func() {
			client.CreateReturnsOnCall(1, k8serrors.NewAlreadyExists(schema.GroupResource{}, "potato"))
		})

		It("does not fail", func() {
			Expect(desireErr).NotTo(HaveOccurred())
		})
	})

	When("creating the statefulset fails", func() {
		BeforeEach(func() {
			client.CreateReturns(errors.New("potato"))
		})

		It("propagates the error", func() {
			Expect(desireErr).To(MatchError(ContainSubstring("potato")))
		})
	})

	When("the app references a private docker image", func() {
		var stsetCreateErr error

		BeforeEach(func() {
			stsetCreateErr = nil
			lrp.Spec.PrivateRegistry = &eiriniv1.PrivateRegistry{
				Username: "user",
				Password: "password",
			}

			client.CreateStub = func(_ context.Context, obj k8sclient.Object, _ ...k8sclient.CreateOption) error {
				secret, ok := obj.(*corev1.Secret)
				if ok {
					secret.Name = "private-registry-1234"
				}

				_, ok = obj.(*appsv1.StatefulSet)
				if ok {
					return stsetCreateErr
				}

				return nil
			}
		})

		It("should create a private repo secret containing the private repo credentials", func() {
			Expect(client.CreateCallCount()).To(Equal(2))
			_, obj, _ := client.CreateArgsForCall(0)

			Expect(obj).To(BeAssignableToTypeOf(&corev1.Secret{}))
			actualSecret := obj.(*corev1.Secret)

			Expect(actualSecret.Namespace).To(Equal("the-namespace"))
			Expect(actualSecret.GenerateName).To(Equal("private-registry-"))
			Expect(actualSecret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			Expect(actualSecret.StringData).To(
				HaveKeyWithValue(
					".dockerconfigjson",
					fmt.Sprintf(
						`{"auths":{"gcr.io":{"username":"user","password":"password","auth":"%s"}}}`,
						base64.StdEncoding.EncodeToString([]byte("user:password")),
					),
				),
			)
		})

		It("uses that secret when converting to statefulset", func() {
			Expect(lrpToStatefulSetConverter.ConvertCallCount()).To(Equal(1))
			_, _, actualRegistrySecret := lrpToStatefulSetConverter.ConvertArgsForCall(0)
			Expect(actualRegistrySecret.Name).To(Equal("private-registry-1234"))
			Expect(actualRegistrySecret.Namespace).To(Equal("the-namespace"))
		})

		It("sets the statefulset as the secret owner", func() {
			Expect(client.PatchCallCount()).To(Equal(1))
			_, obj, _, _ := client.PatchArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&corev1.Secret{}))
			patchedSecret := obj.(*corev1.Secret)
			Expect(patchedSecret.OwnerReferences).To(HaveLen(1))
			Expect(patchedSecret.OwnerReferences[0].Kind).To(Equal("StatefulSet"))
			Expect(patchedSecret.OwnerReferences[0].Name).To(HavePrefix("baldur-space-foo"))
		})

		When("creating the statefulset fails", func() {
			BeforeEach(func() {
				stsetCreateErr = errors.New("potato")
			})

			It("deletes the secret", func() {
				Expect(client.DeleteCallCount()).To(Equal(1))
				_, obj, _ := client.DeleteArgsForCall(0)
				Expect(obj).To(BeAssignableToTypeOf(&corev1.Secret{}))
				actualSecret := obj.(*corev1.Secret)
				Expect(actualSecret.Namespace).To(Equal("the-namespace"))
				Expect(actualSecret.Name).To(Equal("private-registry-1234"))
			})

			When("deleting the secret fails", func() {
				BeforeEach(func() {
					client.DeleteReturns(errors.New("delete-secret-failed"))
				})

				It("returns a statefulset creation error and a note that the secret is not cleaned up", func() {
					Expect(desireErr).To(MatchError(And(ContainSubstring("potato"), ContainSubstring("delete-secret-failed"))))
				})
			})
		})

		When("setting the statefulset as a secret owner fails", func() {
			BeforeEach(func() {
				client.PatchReturns(errors.New("set-owner-failed"))
			})

			It("returns an error", func() {
				Expect(desireErr).To(MatchError(ContainSubstring("set-owner-failed")))
			})
		})
	})
})

func expectedValFrom(fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: "",
			FieldPath:  fieldPath,
		},
	}
}
