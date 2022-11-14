package helpers

import (
	"context"

	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestFixture struct {
	ctx             context.Context
	k8sClient       client.Client
	objectsRegistry map[client.ObjectKey]client.Object
}

func NewTestFixture(k8sClient client.Client) *TestFixture {
	return &TestFixture{
		ctx:             context.Background(),
		k8sClient:       k8sClient,
		objectsRegistry: map[client.ObjectKey]client.Object{},
	}
}

func (f *TestFixture) CreateNamespace(namespaceName string) {
	Expect(f.k8sClient.Create(f.ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	})).To(Succeed())
}

func (f *TestFixture) registerObject(obj client.Object) {
	f.objectsRegistry[client.ObjectKeyFromObject(obj)] = obj
}

func RegisterObject[T client.Object](fixture *TestFixture, obj T) T {
	fixture.registerObject(obj)
	return obj
}

func (f *TestFixture) DeregisterObject(obj ...client.Object) {
	for _, o := range obj {
		delete(f.objectsRegistry, client.ObjectKeyFromObject(o))
	}
}

func (f *TestFixture) CreateAllRegisteredObjects() {
	for _, o := range f.objectsRegistry {
		f.SyncCreate(o)
	}
}

// SyncCreateObject creates the object and waits until the object can be
// retrieved. This ensures that the newly created object is available in the
// client cache.
// `AlreadyExists` errors are ignored
// If the object has `Status`, then it is patched so that desired status is also set
func (f *TestFixture) SyncCreate(obj client.Object) {
	unstructuredOriginal, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	Expect(err).NotTo(HaveOccurred())

	createErr := f.k8sClient.Create(f.ctx, obj)
	Expect(client.IgnoreAlreadyExists(createErr)).NotTo(HaveOccurred(), "failed to create %q", client.ObjectKeyFromObject(obj))

	Eventually(func(g Gomega) {
		g.Expect(f.k8sClient.Get(f.ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}).Should(Succeed())

	if desiredStatus, hasStatus := unstructuredOriginal["status"]; hasStatus {
		f.syncPatchStatus(obj, desiredStatus)
	}
}

func (f *TestFixture) syncPatchStatus(obj client.Object, desiredStatus interface{}) {
	k8s.Patch(f.ctx, f.k8sClient, obj, func() {
		unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		Expect(err).NotTo(HaveOccurred())
		unstructured["status"] = desiredStatus
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured, obj)).To(Succeed())
	})

	Eventually(func(g Gomega) {
		g.Expect(f.k8sClient.Get(f.ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(unstructured["status"]).To(Equal(desiredStatus))
	}).Should(Succeed())
}
