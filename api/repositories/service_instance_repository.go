package repositories

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CFServiceInstanceGUIDLabel  = "korifi.cloudfoundry.org/service-instance-guid"
	ServiceInstanceResourceType = "Service Instance"
)

type NamespaceGetter interface {
	GetNamespaceForServiceInstance(ctx context.Context, guid string) (string, error)
}

type ServiceInstanceRepo struct {
	klient        Klient
	awaiter       Awaiter[*korifiv1alpha1.CFServiceInstance]
	sorter        ServiceInstanceSorter
	rootNamespace string
}

//counterfeiter:generate -o fake -fake-name ServiceInstanceSorter . ServiceInstanceSorter
type ServiceInstanceSorter interface {
	Sort(records []ServiceInstanceRecord, order string) []ServiceInstanceRecord
}

type serviceInstanceSorter struct {
	sorter *compare.Sorter[ServiceInstanceRecord]
}

func NewServiceInstanceSorter() *serviceInstanceSorter {
	return &serviceInstanceSorter{
		sorter: compare.NewSorter(ServiceInstanceComparator),
	}
}

func (s *serviceInstanceSorter) Sort(records []ServiceInstanceRecord, order string) []ServiceInstanceRecord {
	return s.sorter.Sort(records, order)
}

func ServiceInstanceComparator(fieldName string) func(ServiceInstanceRecord, ServiceInstanceRecord) int {
	return func(s1, s2 ServiceInstanceRecord) int {
		switch fieldName {
		case "created_at":
			return tools.CompareTimePtr(&s1.CreatedAt, &s2.CreatedAt)
		case "-created_at":
			return tools.CompareTimePtr(&s2.CreatedAt, &s1.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(s1.UpdatedAt, s2.UpdatedAt)
		case "-updated_at":
			return tools.CompareTimePtr(s2.UpdatedAt, s1.UpdatedAt)
		case "name":
			return strings.Compare(s1.Name, s2.Name)
		case "-name":
			return strings.Compare(s2.Name, s1.Name)
		}
		return 0
	}
}

func NewServiceInstanceRepo(
	klient Klient,
	awaiter Awaiter[*korifiv1alpha1.CFServiceInstance],
	sorter ServiceInstanceSorter,
	rootNamespace string,
) *ServiceInstanceRepo {
	return &ServiceInstanceRepo{
		klient:        klient,
		awaiter:       awaiter,
		sorter:        sorter,
		rootNamespace: rootNamespace,
	}
}

type CreateUPSIMessage struct {
	Name        string
	SpaceGUID   string
	Credentials map[string]any
	Tags        []string
	Labels      map[string]string
	Annotations map[string]string
}

type CreateManagedSIMessage struct {
	Name        string
	SpaceGUID   string
	PlanGUID    string
	Parameters  map[string]any
	Tags        []string
	Labels      map[string]string
	Annotations map[string]string
}

type PatchServiceInstanceMessage struct {
	GUID        string
	SpaceGUID   string
	Name        *string
	Credentials *map[string]any
	Tags        *[]string
	MetadataPatch
}

func (p PatchServiceInstanceMessage) Apply(cfServiceInstance *korifiv1alpha1.CFServiceInstance) {
	if p.Name != nil {
		cfServiceInstance.Spec.DisplayName = *p.Name
	}
	if p.Tags != nil {
		cfServiceInstance.Spec.Tags = *p.Tags
	}
	p.MetadataPatch.Apply(cfServiceInstance)
}

type ListServiceInstanceMessage struct {
	Names         []string
	SpaceGUIDs    []string
	GUIDs         []string
	Type          string
	LabelSelector string
	OrderBy       string
	PlanGUIDs     []string
}

func (m *ListServiceInstanceMessage) matches(serviceInstance korifiv1alpha1.CFServiceInstance) bool {
	return tools.EmptyOrContains(m.Names, serviceInstance.Spec.DisplayName) &&
		tools.EmptyOrContains(m.GUIDs, serviceInstance.Name) &&
		tools.EmptyOrContains(m.PlanGUIDs, serviceInstance.Spec.PlanGUID) &&
		tools.EmptyOrContains(m.SpaceGUIDs, serviceInstance.Namespace) &&
		tools.ZeroOrEquals(korifiv1alpha1.InstanceType(m.Type), serviceInstance.Spec.Type)
}

type DeleteServiceInstanceMessage struct {
	GUID  string
	Purge bool
}

type ServiceInstanceRecord struct {
	Name             string
	GUID             string
	SpaceGUID        string
	PlanGUID         string
	Tags             []string
	Type             string
	Labels           map[string]string
	Annotations      map[string]string
	CreatedAt        time.Time
	UpdatedAt        *time.Time
	DeletedAt        *time.Time
	LastOperation    korifiv1alpha1.LastOperation
	Ready            bool
	MaintenanceInfo  MaintenanceInfo
	UpgradeAvailable bool
}

func (r ServiceInstanceRecord) Relationships() map[string]string {
	relationships := map[string]string{
		"space": r.SpaceGUID,
	}
	if r.Type == korifiv1alpha1.ManagedType {
		relationships["service_plan"] = r.PlanGUID
	}

	return relationships
}

func (r *ServiceInstanceRepo) CreateUserProvidedServiceInstance(ctx context.Context, authInfo authorization.Info, message CreateUPSIMessage) (ServiceInstanceRecord, error) {
	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   message.SpaceGUID,
			Labels:      message.Labels,
			Annotations: message.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName: message.Name,
			SecretName:  uuid.NewString(),
			Type:        korifiv1alpha1.UserProvidedType,
			Tags:        message.Tags,
		},
	}
	err := r.klient.Create(ctx, cfServiceInstance)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err = r.createCredentialsSecret(ctx, cfServiceInstance, message.Credentials)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	return cfServiceInstanceToRecord(*cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) CreateManagedServiceInstance(ctx context.Context, authInfo authorization.Info, message CreateManagedSIMessage) (ServiceInstanceRecord, error) {
	planVisible, err := r.servicePlanVisible(ctx, message.PlanGUID, message.SpaceGUID)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.NewUnprocessableEntityError(err, "Invalid service plan. Ensure that the service plan exists, is available, and you have access to it.")
	}

	if !planVisible {
		return ServiceInstanceRecord{}, apierrors.NewUnprocessableEntityError(nil, "Invalid service plan. Ensure that the service plan exists, is available, and you have access to it.")
	}

	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   message.SpaceGUID,
			Labels:      message.Labels,
			Annotations: message.Annotations,
		},
		Spec: korifiv1alpha1.CFServiceInstanceSpec{
			DisplayName: message.Name,
			Type:        korifiv1alpha1.ManagedType,
			PlanGUID:    message.PlanGUID,
			Tags:        message.Tags,
			Parameters: corev1.LocalObjectReference{
				Name: uuid.NewString(),
			},
		},
	}

	err = r.klient.Create(ctx, cfServiceInstance)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err = r.createParametersSecret(ctx, cfServiceInstance, message.Parameters)
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	return cfServiceInstanceToRecord(*cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) createParametersSecret(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance, parameters map[string]any) error {
	parametersData, err := tools.ToParametersSecretData(parameters)
	if err != nil {
		return err
	}

	paramsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceInstance.Namespace,
			Name:      cfServiceInstance.Spec.Parameters.Name,
		},
		Data: parametersData,
	}

	_ = controllerutil.SetOwnerReference(cfServiceInstance, paramsSecret, scheme.Scheme)

	return r.klient.Create(ctx, paramsSecret)
}

func (r *ServiceInstanceRepo) servicePlanVisible(ctx context.Context, planGUID string, spaceGUID string) (bool, error) {
	servicePlan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      planGUID,
			Namespace: r.rootNamespace,
		},
	}
	err := r.klient.Get(ctx, servicePlan)
	if err != nil {
		return false, err
	}

	if servicePlan.Spec.Visibility.Type == korifiv1alpha1.PublicServicePlanVisibilityType {
		return true, nil
	}

	if servicePlan.Spec.Visibility.Type == korifiv1alpha1.AdminServicePlanVisibilityType {
		return false, nil
	}

	space := &korifiv1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name: spaceGUID,
		},
	}

	err = r.klient.Get(ctx, space)
	if err != nil {
		return false, err
	}

	return slices.Contains(servicePlan.Spec.Visibility.Organizations, space.Namespace), nil
}

func (r *ServiceInstanceRepo) PatchServiceInstance(ctx context.Context, authInfo authorization.Info, message PatchServiceInstanceMessage) (ServiceInstanceRecord, error) {
	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.GUID,
		},
	}
	if err := r.klient.Get(ctx, cfServiceInstance); err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	err := r.klient.Patch(ctx, cfServiceInstance, func() error {
		message.Apply(cfServiceInstance)
		return nil
	})
	if err != nil {
		return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	if message.Credentials != nil {
		cfServiceInstance, err = r.migrateLegacyCredentials(ctx, cfServiceInstance)
		if err != nil {
			return ServiceInstanceRecord{}, err
		}
		err = r.patchCredentialsSecret(ctx, cfServiceInstance, *message.Credentials)
		if err != nil {
			return ServiceInstanceRecord{}, apierrors.FromK8sError(err, ServiceInstanceResourceType)
		}
	}

	return cfServiceInstanceToRecord(*cfServiceInstance), nil
}

func (r *ServiceInstanceRepo) migrateLegacyCredentials(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (*korifiv1alpha1.CFServiceInstance, error) {
	cfServiceInstance, err := r.awaiter.AwaitCondition(ctx, r.klient, cfServiceInstance, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return nil, err
	}
	err = r.klient.Patch(ctx, cfServiceInstance, func() error {
		cfServiceInstance.Spec.SecretName = cfServiceInstance.Status.Credentials.Name
		return nil
	})
	if err != nil {
		return nil, apierrors.FromK8sError(err, ServiceInstanceResourceType)
	}

	return cfServiceInstance, nil
}

func (r *ServiceInstanceRepo) patchCredentialsSecret(
	ctx context.Context,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	credentials map[string]any,
) error {
	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
		},
	}

	credentialsSecretData, err := tools.ToCredentialsSecretData(credentials)
	if err != nil {
		return errors.New("failed to marshal credentials for service instance")
	}

	return GetAndPatch(ctx, r.klient, credentialsSecret, func() error {
		credentialsSecret.Data = credentialsSecretData
		return nil
	})
}

func (r *ServiceInstanceRepo) createCredentialsSecret(
	ctx context.Context,
	cfServiceInstance *korifiv1alpha1.CFServiceInstance,
	creds map[string]any,
) error {
	credentialsSecretData, err := tools.ToCredentialsSecretData(creds)
	if err != nil {
		return errors.New("failed to marshal credentials for service instance")
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfServiceInstance.Spec.SecretName,
			Namespace: cfServiceInstance.Namespace,
			Labels: map[string]string{
				CFServiceInstanceGUIDLabel: cfServiceInstance.Name,
			},
		},
		Data: credentialsSecretData,
	}
	_ = controllerutil.SetOwnerReference(cfServiceInstance, credentialsSecret, scheme.Scheme)

	return r.klient.Create(ctx, credentialsSecret)
}

// nolint:dupl
func (r *ServiceInstanceRepo) ListServiceInstances(ctx context.Context, authInfo authorization.Info, message ListServiceInstanceMessage) ([]ServiceInstanceRecord, error) {
	serviceInstanceList := new(korifiv1alpha1.CFServiceInstanceList)
	err := r.klient.List(ctx, serviceInstanceList, WithLabelSelector(message.LabelSelector))
	if err != nil {
		return []ServiceInstanceRecord{}, fmt.Errorf("failed to list service instances: %w",
			apierrors.FromK8sError(err, ServiceInstanceResourceType),
		)
	}

	filteredServiceInstances := itx.FromSlice(serviceInstanceList.Items).Filter(message.matches)
	return r.sorter.Sort(slices.Collect(it.Map(filteredServiceInstances, cfServiceInstanceToRecord)), message.OrderBy), nil
}

func (r *ServiceInstanceRepo) GetServiceInstance(ctx context.Context, authInfo authorization.Info, guid string) (ServiceInstanceRecord, error) {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: guid,
		},
	}
	if err := r.klient.Get(ctx, serviceInstance); err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to get service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	return cfServiceInstanceToRecord(*serviceInstance), nil
}

func (r *ServiceInstanceRepo) GetServiceInstanceCredentials(ctx context.Context, authInfo authorization.Info, instanceGUID string) (map[string]any, error) {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: instanceGUID,
		},
	}
	if err := r.klient.Get(ctx, serviceInstance); err != nil {
		return map[string]any{}, fmt.Errorf("failed to get service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceInstance.Spec.SecretName,
			Namespace: serviceInstance.Namespace,
		},
	}

	if err := r.klient.Get(ctx, credentialsSecret); err != nil {
		return map[string]any{}, fmt.Errorf("failed to get credentials secret for service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	credentials, err := tools.FromCredentialsSecretData(credentialsSecret.Data)
	if err != nil {
		return map[string]any{}, apierrors.NewUnprocessableEntityError(err, fmt.Sprintf("failed to decode credentials secret for service instance: %s", instanceGUID))
	}

	return credentials, nil
}

func (r *ServiceInstanceRepo) DeleteServiceInstance(ctx context.Context, authInfo authorization.Info, message DeleteServiceInstanceMessage) (ServiceInstanceRecord, error) {
	serviceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: message.GUID,
		},
	}

	if err := r.klient.Get(ctx, serviceInstance); err != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to get service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	if message.Purge {
		if err := r.klient.Patch(ctx, serviceInstance, func() error {
			serviceInstance.Annotations = tools.SetMapValue(serviceInstance.Annotations, korifiv1alpha1.DeprovisionWithoutBrokerAnnotation, "true")
			return nil
		}); err != nil {
			return ServiceInstanceRecord{}, fmt.Errorf("failed to remove finalizer for service instance: %s, %w", message.GUID, apierrors.FromK8sError(err, ServiceInstanceResourceType))
		}
	}

	err := r.klient.Delete(ctx, serviceInstance)
	if client.IgnoreNotFound(err) != nil {
		return ServiceInstanceRecord{}, fmt.Errorf("failed to delete service instance: %w", apierrors.FromK8sError(err, ServiceInstanceResourceType))
	}

	return cfServiceInstanceToRecord(*serviceInstance), nil
}

func (r ServiceInstanceRecord) GetResourceType() string {
	return ServiceInstanceResourceType
}

func (r *ServiceInstanceRepo) GetState(ctx context.Context, authInfo authorization.Info, guid string) (ResourceState, error) {
	instanceRecord, err := r.GetServiceInstance(ctx, authInfo, guid)
	if err != nil {
		return ResourceStateUnknown, err
	}

	if instanceRecord.Ready {
		return ResourceStateReady, nil
	}

	return ResourceStateUnknown, nil
}

func (r *ServiceInstanceRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, instanceGUID string) (*time.Time, error) {
	serviceInstance, err := r.GetServiceInstance(ctx, authInfo, instanceGUID)
	if err != nil {
		return nil, err
	}
	return serviceInstance.DeletedAt, nil
}

func cfServiceInstanceToRecord(cfServiceInstance korifiv1alpha1.CFServiceInstance) ServiceInstanceRecord {
	return ServiceInstanceRecord{
		Name:          cfServiceInstance.Spec.DisplayName,
		GUID:          cfServiceInstance.Name,
		SpaceGUID:     cfServiceInstance.Namespace,
		PlanGUID:      cfServiceInstance.Spec.PlanGUID,
		Tags:          cfServiceInstance.Spec.Tags,
		Type:          string(cfServiceInstance.Spec.Type),
		Labels:        cfServiceInstance.Labels,
		Annotations:   cfServiceInstance.Annotations,
		CreatedAt:     cfServiceInstance.CreationTimestamp.Time,
		UpdatedAt:     getLastUpdatedTime(&cfServiceInstance),
		DeletedAt:     golangTime(cfServiceInstance.DeletionTimestamp),
		LastOperation: cfServiceInstance.Status.LastOperation,
		Ready:         isInstanceReady(cfServiceInstance),
		MaintenanceInfo: MaintenanceInfo{
			Version: cfServiceInstance.Status.MaintenanceInfo.Version,
		},
		UpgradeAvailable: cfServiceInstance.Status.UpgradeAvailable,
	}
}

func isInstanceReady(cfServiceInstance korifiv1alpha1.CFServiceInstance) bool {
	if cfServiceInstance.Generation != cfServiceInstance.Status.ObservedGeneration {
		return false
	}

	return meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}
