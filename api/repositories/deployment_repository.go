package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const DeploymentResourceType = "Deployment"

type DeploymentRepo struct {
	userClientFactory  authorization.UserK8sClientFactory
	namespaceRetriever NamespaceRetriever
	rootNamespace      string
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

type DeploymentRecord struct {
	GUID        string
	CreatedAt   string
	UpdatedAt   string
	DropletGUID string
	Status      DeploymentStatus
}

type DeploymentStatus struct {
	Value  string
	Reason string
}

type CreateDeploymentMessage struct {
	AppGUID     string
	DropletGUID string
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

	_, err = controllerutil.CreateOrPatch(ctx, userClient, app, func() error {
		app.Spec.CurrentDropletRef.Name = dropletGUID
		if app.Annotations == nil {
			app.Annotations = map[string]string{}
		}
		app.Annotations["korifi.cloudfoundry.org/restartedAt"] = time.Now().Format(time.RFC3339)
		app.Spec.DesiredState = korifiv1alpha1.StartedState
		return nil
	})
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	deploymentRecord := appToDeploymentRecord(app)
	deploymentRecord.DropletGUID = dropletGUID

	return deploymentRecord, nil
}

func (r *DeploymentRepo) CancelDeployment(ctx context.Context, authInfo authorization.Info, deploymentGUID string) (DeploymentRecord, error) {
	return DeploymentRecord{}, nil
}

func appToDeploymentRecord(cfApp *korifiv1alpha1.CFApp) DeploymentRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfApp.ObjectMeta)
	deploymentRecord := DeploymentRecord{
		GUID:        cfApp.Name,
		CreatedAt:   updatedAtTime,
		UpdatedAt:   updatedAtTime,
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Status: DeploymentStatus{
			Value:  "ACTIVE",
			Reason: "DEPLOYING",
		},
	}

	if meta.IsStatusConditionTrue(cfApp.Status.Conditions, workloads.StatusConditionReady) {
		deploymentRecord.Status = DeploymentStatus{
			Value:  "FINALIZED",
			Reason: "DEPLOYED",
		}
	}

	return deploymentRecord
}
