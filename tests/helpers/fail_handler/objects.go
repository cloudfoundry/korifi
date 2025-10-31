package fail_handler

import (
	"context"
	"fmt"
	"reflect"

	"github.com/onsi/ginkgo/v2"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"go.yaml.in/yaml/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	rest "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme.Scheme))
	utilruntime.Must(corev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(buildv1alpha2.AddToScheme(scheme.Scheme))
}

func PrintObject(k8sClient client.Client, obj client.Object) error {
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		return fmt.Errorf("failed to get object %q: %v", client.ObjectKeyFromObject(obj), err)
	}

	fmt.Fprintf(ginkgo.GinkgoWriter, "\n\n========== %T %s/%s (skipping managed fields) ==========\n", obj, obj.GetNamespace(), obj.GetName())
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{})
	objBytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed marshalling object %v: %v", obj, err)
	}
	fmt.Fprintln(ginkgo.GinkgoWriter, string(objBytes))
	return nil
}

func PrintAllObjects(config *rest.Config, orgGUID string, objs client.ObjectList) {
	k8sClient, err := createK8sClient(config)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create k8s client: %v\n", err)
		return

	}

	objects, err := collectObjects(k8sClient, orgGUID, objs)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to list objects %T: %v\n", objs, err)
		return
	}

	if len(objects) == 0 {
		fmt.Fprintf(ginkgo.GinkgoWriter, "no objects found of type %T\n", objs)
		return
	}

	for _, o := range objects {
		if err := PrintObject(k8sClient, o); err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "failed to print object %s/%s: %v\n", o.GetNamespace(), o.GetName(), err)
		}
	}
}

func collectObjects(k8sClient client.Client, orgGUID string, objectList client.ObjectList) ([]client.Object, error) {
	spaceNamespaces := &corev1.NamespaceList{}
	if err := k8sClient.List(context.Background(), spaceNamespaces, client.MatchingLabels{
		"korifi.cloudfoundry.org/org-guid": orgGUID,
	}); err != nil {
		return nil, fmt.Errorf("failed to list %T: %v", objectList, err)
	}

	objects := []client.Object{}
	for _, ns := range spaceNamespaces.Items {
		if err := k8sClient.List(context.Background(), objectList, client.InNamespace(ns.Name)); err != nil {
			return nil, fmt.Errorf("failed to list %T: %v", objectList, err)
		}

		s := reflect.ValueOf(objectList).Elem().FieldByName("Items")
		for i := 0; i < s.Len(); i++ {
			objects = append(objects, s.Index(i).Addr().Interface().(client.Object))
		}
	}

	return objects, nil
}

func createK8sClient(config *rest.Config) (client.Client, error) {
	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %v", err)
	}

	return k8sClient, nil
}
