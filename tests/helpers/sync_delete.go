package helpers

import (
	"context"

	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SyncDelete(k8sClient client.Client, objs ...client.Object) {
	for _, obj := range objs {
		Expect(k8sClient.Delete(context.Background(), obj)).To(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
			g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "failed to delete %v: %v", client.ObjectKeyFromObject(obj), err)
		}).Should(Succeed())
	}
}
