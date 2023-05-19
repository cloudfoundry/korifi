package cleanup

import (
	"context"
	"sort"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PackageCleaner struct {
	k8sClient        client.Client
	retainedPackages int
}

func NewPackageCleaner(k8sClient client.Client, retainedPackages int) PackageCleaner {
	return PackageCleaner{k8sClient: k8sClient, retainedPackages: retainedPackages}
}

func (c PackageCleaner) Clean(ctx context.Context, app types.NamespacedName) error {
	var cfApp korifiv1alpha1.CFApp
	err := c.k8sClient.Get(ctx, app, &cfApp)
	if err != nil {
		return err
	}

	var currentPackage string
	if cfApp.Spec.CurrentDropletRef.Name != "" {
		var cfBuild korifiv1alpha1.CFBuild
		err = c.k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      cfApp.Spec.CurrentDropletRef.Name,
		}, &cfBuild)
		if err != nil {
			return err
		}
		currentPackage = cfBuild.Spec.PackageRef.Name
	}

	var cfPackages korifiv1alpha1.CFPackageList
	err = c.k8sClient.List(ctx, &cfPackages,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{
			controllers.LabelAppGUID: app.Name,
		},
	)
	if err != nil {
		return err
	}

	var deletablePackages []korifiv1alpha1.CFPackage
	for _, cfPackage := range cfPackages.Items {
		if cfPackage.Name == currentPackage {
			continue
		}
		if !meta.IsStatusConditionTrue(cfPackage.Status.Conditions, shared.StatusConditionReady) {
			continue
		}
		deletablePackages = append(deletablePackages, cfPackage)
	}

	sort.Slice(deletablePackages, func(i, j int) bool {
		return deletablePackages[j].CreationTimestamp.Before(&deletablePackages[i].CreationTimestamp)
	})

	for i := c.retainedPackages; i < len(deletablePackages); i++ {
		err = c.k8sClient.Delete(ctx, &deletablePackages[i])
		if err != nil {
			return err
		}
	}

	return nil
}
