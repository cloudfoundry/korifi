package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	StartedState DesiredState = "STARTED"
	StoppedState DesiredState = "STOPPED"

	CFAppGUIDLabel     string = "korifi.cloudfoundry.org/app-guid"
	AppResourceType    string = "App"
	AppEnvResourceType string = "App Env"
)

type AppRepo struct {
	klient     Klient
	appAwaiter Awaiter[*korifiv1alpha1.CFApp]
	sorter     AppSorter
}

//counterfeiter:generate -o fake -fake-name AppSorter . AppSorter
type AppSorter interface {
	Sort(records []AppRecord, order string) []AppRecord
}

type appSorter struct {
	sorter *compare.Sorter[AppRecord]
}

func NewAppSorter() *appSorter {
	return &appSorter{
		sorter: compare.NewSorter(AppComparator),
	}
}

func (s *appSorter) Sort(records []AppRecord, order string) []AppRecord {
	return s.sorter.Sort(records, order)
}

func AppComparator(fieldName string) func(AppRecord, AppRecord) int {
	return func(a1, a2 AppRecord) int {
		switch fieldName {
		case "", "name":
			return strings.Compare(a1.Name, a2.Name)
		case "-name":
			return strings.Compare(a2.Name, a1.Name)
		case "created_at":
			return tools.CompareTimePtr(&a1.CreatedAt, &a2.CreatedAt)
		case "-created_at":
			return tools.CompareTimePtr(&a2.CreatedAt, &a1.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(a1.UpdatedAt, a2.UpdatedAt)
		case "-updated_at":
			return tools.CompareTimePtr(a2.UpdatedAt, a1.UpdatedAt)
		case "state":
			return strings.Compare(string(a1.State), string(a2.State))
		case "-state":
			return strings.Compare(string(a2.State), string(a1.State))
		}
		return 0
	}
}

func NewAppRepo(
	klient Klient,
	appAwaiter Awaiter[*korifiv1alpha1.CFApp],
	sorter AppSorter,
) *AppRepo {
	return &AppRepo{
		klient:     klient,
		appAwaiter: appAwaiter,
		sorter:     sorter,
	}
}

type AppRecord struct {
	Name                  string
	GUID                  string
	EtcdUID               types.UID
	Revision              string
	SpaceGUID             string
	DropletGUID           string
	Labels                map[string]string
	Annotations           map[string]string
	State                 DesiredState
	Lifecycle             Lifecycle
	CreatedAt             time.Time
	UpdatedAt             *time.Time
	DeletedAt             *time.Time
	IsStaged              bool
	envSecretName         string
	vcapServiceSecretName string
	vcapAppSecretName     string
}

func (a AppRecord) GetResourceType() string {
	return AppResourceType
}

func (a AppRecord) Relationships() map[string]string {
	return map[string]string{
		"space": a.SpaceGUID,
	}
}

type DesiredState string

type Lifecycle struct {
	Type string
	Data LifecycleData
}

type LifecycleData struct {
	Buildpacks []string
	Stack      string
}

type LifecyclePatch struct {
	Type *string
	Data *LifecycleDataPatch
}

type LifecycleDataPatch struct {
	Buildpacks *[]string
	Stack      string
}

type AppEnvVarsRecord struct {
	Name                 string
	AppGUID              string
	SpaceGUID            string
	EnvironmentVariables map[string]string
}

type AppEnvRecord struct {
	AppGUID              string
	SpaceGUID            string
	EnvironmentVariables map[string]string
	SystemEnv            map[string]interface{}
	AppEnv               map[string]interface{}
}

type CurrentDropletRecord struct {
	AppGUID     string
	DropletGUID string
}

type CreateAppMessage struct {
	Name                 string
	SpaceGUID            string
	State                DesiredState
	Lifecycle            Lifecycle
	EnvironmentVariables map[string]string
	Metadata
}

type PatchAppMessage struct {
	AppGUID              string
	SpaceGUID            string
	Name                 string
	Lifecycle            *LifecyclePatch
	EnvironmentVariables map[string]string
	MetadataPatch
}

type DeleteAppMessage struct {
	AppGUID   string
	SpaceGUID string
}

type CreateOrPatchAppEnvVarsMessage struct {
	AppGUID              string
	AppEtcdUID           types.UID
	SpaceGUID            string
	EnvironmentVariables map[string]string
}

type PatchAppEnvVarsMessage struct {
	AppGUID              string
	SpaceGUID            string
	EnvironmentVariables map[string]*string
}

type SetCurrentDropletMessage struct {
	AppGUID     string
	DropletGUID string
	SpaceGUID   string
}

type SetAppDesiredStateMessage struct {
	AppGUID      string
	SpaceGUID    string
	DesiredState string
}

type ListAppsMessage struct {
	Names         []string
	Guids         []string
	SpaceGUIDs    []string
	LabelSelector string
	OrderBy       string
}

func (m *ListAppsMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelSelector(m.LabelSelector),
		WithLabelIn(korifiv1alpha1.GUIDLabelKey, m.Guids),
		WithLabelIn(korifiv1alpha1.SpaceGUIDKey, m.SpaceGUIDs),
		WithLabelIn(korifiv1alpha1.CFAppDisplayNameKey, m.Names),
	}
}

func (f *AppRepo) GetApp(ctx context.Context, authInfo authorization.Info, appGUID string) (AppRecord, error) {
	app := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: appGUID,
		},
	}
	err := f.klient.Get(ctx, app)
	if err != nil {
		return AppRecord{}, fmt.Errorf("failed to get app: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	return cfAppToAppRecord(*app), nil
}

func (f *AppRepo) CreateApp(ctx context.Context, authInfo authorization.Info, appCreateMessage CreateAppMessage) (AppRecord, error) {
	cfApp := appCreateMessage.toCFApp()
	err := f.klient.Create(ctx, &cfApp)
	if err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == validation.DuplicateNameErrorType {
				return AppRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return AppRecord{}, apierrors.FromK8sError(err, AppResourceType)
	}

	envSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfApp.Spec.EnvSecretName,
			Namespace: cfApp.Namespace,
			Labels: map[string]string{
				CFAppGUIDLabel: cfApp.Name,
			},
		},
		StringData: appCreateMessage.EnvironmentVariables,
	}
	_ = controllerutil.SetOwnerReference(&cfApp, envSecret, scheme.Scheme)

	err = f.klient.Create(ctx, envSecret)
	if err != nil {
		return AppRecord{}, apierrors.FromK8sError(err, AppResourceType)
	}

	return cfAppToAppRecord(cfApp), nil
}

func (f *AppRepo) PatchApp(ctx context.Context, authInfo authorization.Info, appPatchMessage PatchAppMessage) (AppRecord, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: appPatchMessage.SpaceGUID,
			Name:      appPatchMessage.AppGUID,
		},
	}

	err := GetAndPatch(ctx, f.klient, cfApp, func() error {
		appPatchMessage.Apply(cfApp)
		return nil
	})
	if err != nil {
		return AppRecord{}, apierrors.FromK8sError(err, AppResourceType)
	}

	envSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfApp.Namespace,
			Name:      cfApp.Spec.EnvSecretName,
		},
	}
	err = GetAndPatch(ctx, f.klient, envSecret, func() error {
		if envSecret.Data == nil {
			envSecret.Data = map[string][]byte{}
		}
		for k, v := range appPatchMessage.EnvironmentVariables {
			envSecret.Data[k] = []byte(v)
		}
		return nil
	})
	if err != nil {
		return AppRecord{}, apierrors.FromK8sError(err, AppResourceType)
	}
	return cfAppToAppRecord(*cfApp), nil
}

func (f *AppRepo) ListApps(ctx context.Context, authInfo authorization.Info, message ListAppsMessage) ([]AppRecord, error) {
	appList := &korifiv1alpha1.CFAppList{}
	err := f.klient.List(ctx, appList, message.toListOptions()...)
	if err != nil {
		return []AppRecord{}, fmt.Errorf("failed to list apps: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	appRecords := it.Map(itx.FromSlice(appList.Items), cfAppToAppRecord)

	return f.sorter.Sort(slices.Collect(appRecords), message.OrderBy), nil
}

func (f *AppRepo) PatchAppEnvVars(ctx context.Context, authInfo authorization.Info, message PatchAppEnvVarsMessage) (AppEnvVarsRecord, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.AppGUID,
		},
	}
	err := f.klient.Get(ctx, cfApp)
	if err != nil {
		return AppEnvVarsRecord{}, apierrors.FromK8sError(err, AppEnvResourceType)
	}

	secretObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfApp.Spec.EnvSecretName,
			Namespace: message.SpaceGUID,
		},
	}

	err = GetAndPatch(ctx, f.klient, &secretObj, func() error {
		if secretObj.Data == nil {
			secretObj.Data = map[string][]byte{}
		}
		for k, v := range message.EnvironmentVariables {
			if v == nil {
				delete(secretObj.Data, k)
			} else {
				secretObj.Data[k] = []byte(*v)
			}
		}

		return nil
	})
	if err != nil {
		return AppEnvVarsRecord{}, apierrors.FromK8sError(err, AppEnvResourceType)
	}

	return appEnvVarsSecretToRecord(secretObj), nil
}

func (f *AppRepo) SetCurrentDroplet(ctx context.Context, authInfo authorization.Info, message SetCurrentDropletMessage) (CurrentDropletRecord, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.AppGUID,
			Namespace: message.SpaceGUID,
		},
	}

	err := GetAndPatch(ctx, f.klient, cfApp, func() error {
		cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: message.DropletGUID}
		return nil
	})
	if err != nil {
		return CurrentDropletRecord{}, fmt.Errorf("failed to set app droplet: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	_, err = f.appAwaiter.AwaitCondition(ctx, f.klient, cfApp, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return CurrentDropletRecord{}, fmt.Errorf("failed to await the app staged condition: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	return CurrentDropletRecord{
		AppGUID:     message.AppGUID,
		DropletGUID: message.DropletGUID,
	}, nil
}

func (f *AppRepo) SetAppDesiredState(ctx context.Context, authInfo authorization.Info, message SetAppDesiredStateMessage) (AppRecord, error) {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.AppGUID,
			Namespace: message.SpaceGUID,
		},
	}

	err := GetAndPatch(ctx, f.klient, cfApp, func() error {
		cfApp.Spec.DesiredState = korifiv1alpha1.AppState(message.DesiredState)
		return nil
	})
	if err != nil {
		return AppRecord{}, fmt.Errorf("failed to set app desired state: %w", apierrors.FromK8sError(err, AppResourceType))
	}

	_, err = f.appAwaiter.AwaitState(ctx, f.klient, cfApp, func(a *korifiv1alpha1.CFApp) error {
		if _, readyConditionErr := f.appAwaiter.AwaitCondition(ctx, f.klient, a, korifiv1alpha1.StatusConditionReady); err != nil {
			return readyConditionErr
		}

		if a.Spec.DesiredState != korifiv1alpha1.AppState(message.DesiredState) ||
			a.Status.ActualState != korifiv1alpha1.AppState(message.DesiredState) {
			return fmt.Errorf("desired state %q not reached; actual state: %q", message.DesiredState, a.Status.ActualState)
		}

		return nil
	})
	if err != nil {
		return AppRecord{}, apierrors.FromK8sError(err, AppResourceType)
	}

	return cfAppToAppRecord(*cfApp), nil
}

func (f *AppRepo) DeleteApp(ctx context.Context, authInfo authorization.Info, message DeleteAppMessage) error {
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.AppGUID,
			Namespace: message.SpaceGUID,
		},
	}

	return apierrors.FromK8sError(
		f.klient.Delete(ctx, cfApp, client.PropagationPolicy(metav1.DeletePropagationForeground)),
		AppResourceType,
	)
}

func (f *AppRepo) GetAppEnv(ctx context.Context, authInfo authorization.Info, appGUID string) (AppEnvRecord, error) {
	app, err := f.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return AppEnvRecord{}, err
	}

	appEnvVarMap := map[string]string{}
	if app.envSecretName != "" {
		appEnvVarSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: app.SpaceGUID,
				Name:      app.envSecretName,
			},
		}
		err = f.klient.Get(ctx, appEnvVarSecret)
		if err != nil {
			return AppEnvRecord{}, fmt.Errorf("error finding environment variable Secret %q for App %q: %w",
				app.envSecretName,
				app.GUID,
				apierrors.FromK8sError(err, AppEnvResourceType))
		}
		appEnvVarMap = convertByteSliceValuesToStrings(appEnvVarSecret.Data)
	}

	systemEnvMap, err := f.getSystemEnv(ctx, app)
	if err != nil {
		return AppEnvRecord{}, err
	}

	appEnvMap, err := f.getAppEnv(ctx, app)
	if err != nil {
		return AppEnvRecord{}, err
	}

	appEnvRecord := AppEnvRecord{
		AppGUID:              appGUID,
		SpaceGUID:            app.SpaceGUID,
		EnvironmentVariables: appEnvVarMap,
		SystemEnv:            systemEnvMap,
		AppEnv:               appEnvMap,
	}

	return appEnvRecord, nil
}

func (f *AppRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, appGUID string) (*time.Time, error) {
	app, err := f.GetApp(ctx, authInfo, appGUID)
	if err != nil {
		return nil, err
	}
	return app.DeletedAt, nil
}

func (f *AppRepo) getSystemEnv(ctx context.Context, app AppRecord) (map[string]any, error) {
	systemEnvMap := map[string]any{}
	if app.vcapServiceSecretName != "" {
		vcapServiceSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: app.SpaceGUID,
				Name:      app.vcapServiceSecretName,
			},
		}
		err := f.klient.Get(ctx, vcapServiceSecret)
		if err != nil {
			return map[string]any{}, fmt.Errorf("error finding VCAP Service Secret %q for App %q: %w",
				app.vcapServiceSecretName,
				app.GUID,
				apierrors.FromK8sError(err, AppEnvResourceType))
		}

		if vcapServicesData, ok := vcapServiceSecret.Data["VCAP_SERVICES"]; ok {
			vcapServicesPresenter := new(env.VCAPServices)
			if err = json.Unmarshal(vcapServicesData, &vcapServicesPresenter); err != nil {
				return map[string]any{}, fmt.Errorf("error unmarshalling VCAP Service Secret %q for App %q: %w",
					app.vcapServiceSecretName,
					app.GUID,
					apierrors.FromK8sError(err, AppEnvResourceType))
			}

			if len(*vcapServicesPresenter) > 0 {
				systemEnvMap["VCAP_SERVICES"] = vcapServicesPresenter
			}
		}
	}

	return systemEnvMap, nil
}

func (f *AppRepo) getAppEnv(ctx context.Context, app AppRecord) (map[string]any, error) {
	appEnvMap := map[string]any{}
	if app.vcapAppSecretName != "" {
		vcapAppSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: app.SpaceGUID,
				Name:      app.vcapAppSecretName,
			},
		}
		err := f.klient.Get(ctx, vcapAppSecret)
		if err != nil {
			return map[string]any{}, fmt.Errorf("error finding VCAP Application Secret %q for App %q: %w",
				app.vcapAppSecretName,
				app.GUID,
				apierrors.FromK8sError(err, AppEnvResourceType))
		}

		if vcapAppDataBytes, ok := vcapAppSecret.Data["VCAP_APPLICATION"]; ok {
			appData := map[string]any{}
			if err = json.Unmarshal(vcapAppDataBytes, &appData); err != nil {
				return map[string]any{}, fmt.Errorf("error unmarshalling VCAP Application Secret %q for App %q: %w",
					app.vcapAppSecretName,
					app.GUID,
					apierrors.FromK8sError(err, AppEnvResourceType))
			}
			appEnvMap["VCAP_APPLICATION"] = appData
		}
	}

	return appEnvMap, nil
}

func (m *CreateAppMessage) toCFApp() korifiv1alpha1.CFApp {
	return korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:   m.Name,
			DesiredState:  korifiv1alpha1.AppState(m.State),
			EnvSecretName: uuid.NewString(),
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: korifiv1alpha1.LifecycleType(m.Lifecycle.Type),
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: m.Lifecycle.Data.Buildpacks,
					Stack:      m.Lifecycle.Data.Stack,
				},
			},
		},
	}
}

func (m *PatchAppMessage) Apply(app *korifiv1alpha1.CFApp) {
	if m.Name != "" {
		app.Spec.DisplayName = m.Name
	}

	if m.Lifecycle != nil {
		if m.Lifecycle.Type != nil {
			app.Spec.Lifecycle.Type = korifiv1alpha1.LifecycleType(*m.Lifecycle.Type)
		}

		if m.Lifecycle.Data.Buildpacks != nil {
			app.Spec.Lifecycle.Data.Buildpacks = *m.Lifecycle.Data.Buildpacks
		}

		if m.Lifecycle.Data.Stack != "" {
			app.Spec.Lifecycle.Data.Stack = m.Lifecycle.Data.Stack
		}
	}

	m.MetadataPatch.Apply(app)
}

func cfAppToAppRecord(cfApp korifiv1alpha1.CFApp) AppRecord {
	return AppRecord{
		GUID:        cfApp.Name,
		EtcdUID:     cfApp.GetUID(),
		Revision:    getLabelOrAnnotation(cfApp.GetAnnotations(), korifiv1alpha1.CFAppRevisionKey),
		Name:        cfApp.Spec.DisplayName,
		SpaceGUID:   cfApp.Namespace,
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Labels:      cfApp.Labels,
		Annotations: cfApp.Annotations,
		State:       DesiredState(cfApp.Spec.DesiredState),
		Lifecycle: Lifecycle{
			Type: string(cfApp.Spec.Lifecycle.Type),
			Data: LifecycleData{
				Buildpacks: cfApp.Spec.Lifecycle.Data.Buildpacks,
				Stack:      cfApp.Spec.Lifecycle.Data.Stack,
			},
		},
		CreatedAt:             cfApp.CreationTimestamp.Time,
		UpdatedAt:             getLastUpdatedTime(&cfApp),
		DeletedAt:             golangTime(cfApp.DeletionTimestamp),
		IsStaged:              cfApp.Spec.CurrentDropletRef.Name != "",
		envSecretName:         cfApp.Spec.EnvSecretName,
		vcapServiceSecretName: cfApp.Status.VCAPServicesSecretName,
		vcapAppSecretName:     cfApp.Status.VCAPApplicationSecretName,
	}
}

func appEnvVarsSecretToRecord(envVars corev1.Secret) AppEnvVarsRecord {
	appGUID := strings.TrimSuffix(envVars.Name, "-env")
	return AppEnvVarsRecord{
		Name:                 envVars.Name,
		AppGUID:              appGUID,
		SpaceGUID:            envVars.Namespace,
		EnvironmentVariables: convertByteSliceValuesToStrings(envVars.Data),
	}
}

func convertByteSliceValuesToStrings(inputMap map[string][]byte) map[string]string {
	return maps.Collect(it.Map2(maps.All(inputMap), func(k string, v []byte) (string, string) {
		return k, string(v)
	}))
}
