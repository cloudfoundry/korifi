package repositories

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/version"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DeploymentResourceType = "Deployment"

type DeploymentRepo struct {
	klient Klient
}

type DeploymentRecord struct {
	GUID        string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	DropletGUID string
	Status      DeploymentStatus
}

func (r DeploymentRecord) Relationships() map[string]string {
	return map[string]string{
		"app": r.GUID,
	}
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

type ListDeploymentsMessage struct {
	AppGUIDs     []string
	StatusValues []DeploymentStatusValue
	OrderBy      string
	Pagination   Pagination
}

func (m ListDeploymentsMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.GUIDLabelKey, m.AppGUIDs),
		WithLabelIn(korifiv1alpha1.CFAppDeploymentStatusKey, slices.Collect(it.Map(slices.Values(m.StatusValues), func(s DeploymentStatusValue) string {
			return string(s)
		}))),
		WithOrdering(m.OrderBy),
		WithPaging(m.Pagination),
	}
}

func NewDeploymentRepo(
	klient Klient,
) *DeploymentRepo {
	return &DeploymentRepo{
		klient: klient,
	}
}

func (r *DeploymentRepo) GetDeployment(ctx context.Context, authInfo authorization.Info, deploymentGUID string) (DeploymentRecord, error) {
	app := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentGUID,
		},
	}
	err := r.klient.Get(ctx, app)
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	return appToDeploymentRecord(*app)
}

func (r *DeploymentRepo) CreateDeployment(ctx context.Context, authInfo authorization.Info, message CreateDeploymentMessage) (DeploymentRecord, error) {
	app := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: message.AppGUID,
		},
	}
	err := r.klient.Get(ctx, app)
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	if err = r.ensureSupport(ctx, app); err != nil {
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

	err = r.klient.Patch(ctx, app, func() error {
		app.Spec.CurrentDropletRef.Name = dropletGUID
		if app.Annotations == nil {
			app.Annotations = map[string]string{}
		}
		app.Annotations[korifiv1alpha1.CFAppRevisionKey] = newRev
		app.Spec.DesiredState = korifiv1alpha1.StartedState

		return nil
	})
	if err != nil {
		return DeploymentRecord{}, apierrors.FromK8sError(err, DeploymentResourceType)
	}

	return appToDeploymentRecord(*app)
}

func (r *DeploymentRepo) ListDeployments(ctx context.Context, authInfo authorization.Info, message ListDeploymentsMessage) (ListResult[DeploymentRecord], error) {
	appList := &korifiv1alpha1.CFAppList{}
	pageInfo, err := r.klient.List(ctx, appList, message.toListOptions()...)
	if err != nil {
		return ListResult[DeploymentRecord]{}, fmt.Errorf("failed to list apps: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	records, err := it.TryCollect(it.MapError(slices.Values(appList.Items), appToDeploymentRecord))
	if err != nil {
		return ListResult[DeploymentRecord]{}, fmt.Errorf("failed to list deployments: %w", apierrors.FromK8sError(err, DeploymentResourceType))
	}

	return ListResult[DeploymentRecord]{
		Records:  records,
		PageInfo: pageInfo,
	}, nil
}

func bumpAppRev(appRev string) (string, error) {
	r, err := strconv.Atoi(appRev)
	if err != nil {
		return "", err
	}

	return strconv.Itoa(r + 1), nil
}

func appToDeploymentRecord(cfApp korifiv1alpha1.CFApp) (DeploymentRecord, error) {
	createdAt, updatedAt, err := getCreatedUpdatedAt(&cfApp)
	if err != nil {
		return DeploymentRecord{}, err
	}

	return DeploymentRecord{
		GUID:        cfApp.Name,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Status:      appToDeploymentStatus(cfApp),
	}, nil
}

func appToDeploymentStatus(cfapp korifiv1alpha1.CFApp) DeploymentStatus {
	deploymentStatusValue := cfapp.Labels[korifiv1alpha1.CFAppDeploymentStatusKey]

	if deploymentStatusValue == korifiv1alpha1.DeploymentStatusValueFinalized {
		return DeploymentStatus{
			Value:  DeploymentStatusValueFinalized,
			Reason: DeploymentStatusReasonDeployed,
		}
	}

	return DeploymentStatus{
		Value:  DeploymentStatusValueActive,
		Reason: DeploymentStatusReasonDeploying,
	}
}

func (r *DeploymentRepo) ensureSupport(ctx context.Context, app *korifiv1alpha1.CFApp) error {
	log := logr.FromContextOrDiscard(ctx).WithName("repo.deployment.ensureSupport")

	var appWorkloadsList korifiv1alpha1.AppWorkloadList
	_, err := r.klient.List(ctx, &appWorkloadsList, InNamespace(app.Namespace), WithLabel(korifiv1alpha1.CFAppGUIDLabelKey, app.Name))
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
