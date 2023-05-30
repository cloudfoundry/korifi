package repositories

import (
	"context"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/version"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DeploymentResourceType = "Deployment"

type DeploymentRepo struct {
	userClientFactory  authorization.UserK8sClientFactory
	namespaceRetriever NamespaceRetriever
}

type DeploymentRecord struct {
	GUID        string
	CreatedAt   string
	UpdatedAt   string
	DropletGUID string
	Status      DeploymentStatus
}

type DeploymentStatusValue string

const (
	DeploymentStatusValueActive    DeploymentStatusValue = "ACTIVE"
	DeploymentStatusValueFinalized DeploymentStatusValue = "FINALIZED"
)

type DeploymentStatusReason string

const (
	DeploymentStatusReasonDeploying DeploymentStatusReason = "DEPLOYING"
	DeploymentStatusReasonDeployed  DeploymentStatusReason = "DEPLOYED"
)

type DeploymentStatus struct {
	Value  DeploymentStatusValue
	Reason DeploymentStatusReason
}

type CreateDeploymentMessage struct {
	AppGUID     string
	DropletGUID string
}

func NewDeploymentRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
) *DeploymentRepo {
	return &DeploymentRepo{
		userClientFactory:  userClientFactory,
		namespaceRetriever: namespaceRetriever,
	}
}

func (r *DeploymentRepo) GetDeployment(ctx context.Context, authInfo authorization.Info, deploymentGUID string) (DeploymentRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, deploymentGUID, AppResourceType)
	if err != nil {
		return DeploymentRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("get-deployment failed to create user client: %w", err)
	}

	app := &korifiv1alpha1.CFApp{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: deploymentGUID}, app)
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	return appToDeploymentRecord(app), nil
}

func (r *DeploymentRepo) CreateDeployment(ctx context.Context, authInfo authorization.Info, message CreateDeploymentMessage) (DeploymentRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, message.AppGUID, AppResourceType)
	if err != nil {
		return DeploymentRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("create-deployment failed to create user client: %w", err)
	}

	app := &korifiv1alpha1.CFApp{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: message.AppGUID}, app)
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	if err = ensureSupport(ctx, userClient, app); err != nil {
		return DeploymentRecord{}, err
	}

	dropletGUID := app.Spec.CurrentDropletRef.Name
	if message.DropletGUID != "" {
		dropletGUID = message.DropletGUID
	}

	appRev := app.Annotations[korifiv1alpha1.CFAppRevisionKey]
	newRev, err := bumpAppRev(appRev)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("expected app-rev to be an integer: %w", err)
	}

	err = k8s.PatchResource(ctx, userClient, app, func() {
		app.Spec.CurrentDropletRef.Name = dropletGUID
		if app.Annotations == nil {
			app.Annotations = map[string]string{}
		}
		app.Annotations[korifiv1alpha1.CFAppRevisionKey] = newRev
		app.Spec.DesiredState = korifiv1alpha1.StartedState
	})
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	return appToDeploymentRecord(app), nil
}

func bumpAppRev(appRev string) (string, error) {
	r, err := strconv.Atoi(appRev)
	if err != nil {
		return "", err
	}

	return strconv.Itoa(r + 1), nil
}

func appToDeploymentRecord(cfApp *korifiv1alpha1.CFApp) DeploymentRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfApp.ObjectMeta)
	deploymentRecord := DeploymentRecord{
		GUID:        cfApp.Name,
		CreatedAt:   updatedAtTime,
		UpdatedAt:   updatedAtTime,
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Status: DeploymentStatus{
			Value:  DeploymentStatusValueActive,
			Reason: DeploymentStatusReasonDeploying,
		},
	}

	if meta.IsStatusConditionTrue(cfApp.Status.Conditions, shared.StatusConditionReady) {
		deploymentRecord.Status = DeploymentStatus{
			Value:  DeploymentStatusValueFinalized,
			Reason: DeploymentStatusReasonDeployed,
		}
	}

	return deploymentRecord
}

func ensureSupport(ctx context.Context, userClient client.Client, app *korifiv1alpha1.CFApp) error {
	log := logr.FromContextOrDiscard(ctx).WithName("repo.deployment.ensureSupport")

	var appWorkloadsList korifiv1alpha1.AppWorkloadList
	err := userClient.List(ctx, &appWorkloadsList, client.InNamespace(app.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: app.Name,
	})
	if err != nil {
		return apierrors.FromK8sError(err, DeploymentResourceType)
	}

	checker := version.NewChecker("v0.7.1")
	for i := range appWorkloadsList.Items {
		appWorkload := appWorkloadsList.Items[i]
		newer, err := checker.ObjectIsNewer(&appWorkload)
		if newer {
			continue
		}

		if err != nil {
			log.Info("failed comparining version of appWorkload",
				"ns", appWorkload.Namespace,
				"name", appWorkload.Name,
				"version", appWorkload.Annotations[version.KorifiCreationVersionKey],
			)
		}
		return apierrors.NewUnprocessableEntityError(nil, "App instances created with an older version of Korifi can't use the rolling strategy. Please restart/restage/re-push app before using the rolling strategy")
	}

	return nil
}
