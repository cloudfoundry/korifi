package cleanup

import (
	"context"
	"sort"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BuildCleaner struct {
	k8sClient      client.Client
	retainedBuilds int
}

func NewBuildCleaner(k8sClient client.Client, retainedBuilds int) BuildCleaner {
	return BuildCleaner{k8sClient: k8sClient, retainedBuilds: retainedBuilds}
}

func (c BuildCleaner) Clean(ctx context.Context, app types.NamespacedName) error {
	var cfApp korifiv1alpha1.CFApp
	err := c.k8sClient.Get(ctx, app, &cfApp)
	if err != nil {
		return err
	}

	var cfBuilds korifiv1alpha1.CFBuildList
	err = c.k8sClient.List(ctx, &cfBuilds,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{
			controllers.LabelAppGUID: app.Name,
		},
	)
	if err != nil {
		return err
	}

	var deletableBuilds []korifiv1alpha1.CFBuild
	for _, cfBuild := range cfBuilds.Items {
		if cfBuild.Name == cfApp.Spec.CurrentDropletRef.Name {
			continue
		}
		if !meta.IsStatusConditionTrue(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType) {
			continue
		}
		deletableBuilds = append(deletableBuilds, cfBuild)
	}

	sort.Slice(deletableBuilds, func(i, j int) bool {
		return deletableBuilds[j].CreationTimestamp.Before(&deletableBuilds[i].CreationTimestamp)
	})

	for i := c.retainedBuilds; i < len(deletableBuilds); i++ {
		err = c.k8sClient.Delete(ctx, &deletableBuilds[i])
		if err != nil {
			return err
		}
	}

	return nil
}
