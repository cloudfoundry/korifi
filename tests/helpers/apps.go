package helpers

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetInClusterURL(appGUID string) string {
	GinkgoHelper()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	services := &corev1.ServiceList{}
	Expect(k8sClient.List(context.Background(), services, client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: appGUID,
	})).To(Succeed())
	Expect(services.Items).To(HaveLen(1))

	appService := services.Items[0]
	return fmt.Sprintf("http://%s.%s:8080", appService.Name, appService.Namespace)
}
