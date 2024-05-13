package cleanup

import (
	"context"
	"fmt"
	"sort"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"github.com/go-logr/logr"
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
	log := logr.FromContextOrDiscard(ctx).WithName("BuildCleaner").WithValues("app", app)

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
	log.Info("processing builds", "count", len(cfBuilds.Items))
	for _, cfBuild := range cfBuilds.Items {
		if cfBuild.Name == cfApp.Spec.CurrentDropletRef.Name {
			continue
		}
		if !meta.IsStatusConditionTrue(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType) {
			continue
		}
		log.Info("found deletable build",
			"buildGUID", cfBuild.Name,
			"appCurrentDroplet", cfApp.Spec.CurrentDropletRef.Name,
			"succeedCondition", fmt.Sprintf("%v", meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)))
		deletableBuilds = append(deletableBuilds, cfBuild)
	}

	sort.Slice(deletableBuilds, func(i, j int) bool {
		return deletableBuilds[j].CreationTimestamp.Before(&deletableBuilds[i].CreationTimestamp)
	})

	for i := c.retainedBuilds; i < len(deletableBuilds); i++ {
		log.Info("deleting deletable build", "buildGUID", deletableBuilds[i].Name)
		err = c.k8sClient.Delete(ctx, &deletableBuilds[i])
		if err != nil {
			return err
		}
	}

	return nil
}
