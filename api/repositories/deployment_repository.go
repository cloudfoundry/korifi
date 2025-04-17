package repositories

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/version"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DeploymentResourceType = "Deployment"

type DeploymentRepo struct {
	klient Klient
	sorter DeploymentSorter
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

//counterfeiter:generate -o fake -fake-name DeploymentSorter . DeploymentSorter
type DeploymentSorter interface {
	Sort(records []DeploymentRecord, order string) []DeploymentRecord
}

type deploymentSorter struct {
	sorter *compare.Sorter[DeploymentRecord]
}

func NewDeploymentSorter() *deploymentSorter {
	return &deploymentSorter{
		sorter: compare.NewSorter(DeploymentComparator),
	}
}

func (s *deploymentSorter) Sort(records []DeploymentRecord, order string) []DeploymentRecord {
	return s.sorter.Sort(records, order)
}

func DeploymentComparator(fieldName string) func(DeploymentRecord, DeploymentRecord) int {
	return func(d1, d2 DeploymentRecord) int {
		switch fieldName {
		case "created_at":
			return tools.CompareTimePtr(&d1.CreatedAt, &d2.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(d1.UpdatedAt, d2.UpdatedAt)
		}
		return 0
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
}

func (m ListDeploymentsMessage) matchesApp(app korifiv1alpha1.CFApp) bool {
	return tools.EmptyOrContains(m.AppGUIDs, app.Name)
}

func (m ListDeploymentsMessage) matchesStatusValue(deployment DeploymentRecord) bool {
	return tools.EmptyOrContains(m.StatusValues, deployment.Status.Value)
}

func NewDeploymentRepo(
	klient Klient,
	sorter DeploymentSorter,
) *DeploymentRepo {
	return &DeploymentRepo{
		klient: klient,
		sorter: sorter,
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

	return appToDeploymentRecord(*app), nil
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

	return appToDeploymentRecord(*app), nil
}

func (r *DeploymentRepo) ListDeployments(ctx context.Context, authInfo authorization.Info, message ListDeploymentsMessage) ([]DeploymentRecord, error) {
	appList := &korifiv1alpha1.CFAppList{}
	err := r.klient.List(ctx, appList)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	deploymentRecords := it.Map(itx.FromSlice(appList.Items).Filter(message.matchesApp), appToDeploymentRecord)
	deploymentRecords = it.Filter(deploymentRecords, message.matchesStatusValue)

	return r.sorter.Sort(slices.Collect(deploymentRecords), message.OrderBy), nil
}

func bumpAppRev(appRev string) (string, error) {
	r, err := strconv.Atoi(appRev)
	if err != nil {
		return "", err
	}

	return strconv.Itoa(r + 1), nil
}

func appToDeploymentRecord(cfApp korifiv1alpha1.CFApp) DeploymentRecord {
	deploymentRecord := DeploymentRecord{
		GUID:        cfApp.Name,
		CreatedAt:   cfApp.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(&cfApp),
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Status: DeploymentStatus{
			Value:  DeploymentStatusValueActive,
			Reason: DeploymentStatusReasonDeploying,
		},
	}

	if meta.IsStatusConditionTrue(cfApp.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		deploymentRecord.Status = DeploymentStatus{
			Value:  DeploymentStatusValueFinalized,
			Reason: DeploymentStatusReasonDeployed,
		}
	}

	return deploymentRecord
}

func (r *DeploymentRepo) ensureSupport(ctx context.Context, app *korifiv1alpha1.CFApp) error {
	log := logr.FromContextOrDiscard(ctx).WithName("repo.deployment.ensureSupport")

	var appWorkloadsList korifiv1alpha1.AppWorkloadList
	err := r.klient.List(ctx, &appWorkloadsList, InNamespace(app.Namespace), WithLabel(korifiv1alpha1.CFAppGUIDLabelKey, app.Name))
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
