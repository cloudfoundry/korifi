package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DeploymentResourceType = "Deployment"

type DeploymentRepo struct {
	userClientFactory  authorization.UserK8sClientFactory
	namespaceRetriever NamespaceRetriever
	rootNamespace      string
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
	rootNamespace string,
) *DeploymentRepo {
	return &DeploymentRepo{
		userClientFactory:  userClientFactory,
		namespaceRetriever: namespaceRetriever,
		rootNamespace:      rootNamespace,
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

	dropletGUID := app.Spec.CurrentDropletRef.Name
	if message.DropletGUID != "" {
		dropletGUID = message.DropletGUID
	}

	err = k8s.PatchResource(ctx, userClient, app, func() {
		app.Spec.CurrentDropletRef.Name = dropletGUID
		if app.Annotations == nil {
			app.Annotations = map[string]string{}
		}
		app.Annotations[korifiv1alpha1.StartedAtAnnotation] = time.Now().Format(time.RFC3339)
		app.Spec.DesiredState = korifiv1alpha1.StartedState
	})
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	return appToDeploymentRecord(app), nil
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

	if meta.IsStatusConditionTrue(cfApp.Status.Conditions, workloads.StatusConditionReady) {
		deploymentRecord.Status = DeploymentStatus{
			Value:  DeploymentStatusValueFinalized,
			Reason: DeploymentStatusReasonDeployed,
		}
	}

	return deploymentRecord
}
