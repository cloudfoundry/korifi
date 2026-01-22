package security_group

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/gomega" //lint:ignore ST1001 this is a test file

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Delete(rootNamespace string, securityGroupGUID string) {
	ctx := context.Background()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &korifiv1alpha1.CFSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: rootNamespace,
			Name:      securityGroupGUID,
		},
	}))).To(Succeed())
}
