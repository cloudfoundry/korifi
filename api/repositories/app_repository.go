package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfapps/status,verbs=get

type AppRepo struct{}

const (
	StartedState DesiredState = "STARTED"
	StoppedState DesiredState = "STOPPED"

	Kind            string = "CFApp"
	APIVersion      string = "workloads.cloudfoundry.org/v1alpha1"
	TimestampFormat string = time.RFC3339
	CFAppGUIDLabel  string = "apps.cloudfoundry.org/appGuid"
)

type AppRecord struct {
	Name          string
	GUID          string
	SpaceGUID     string
	DropletGUID   string
	Labels        map[string]string
	Annotations   map[string]string
	State         DesiredState
	Lifecycle     Lifecycle
	EnvSecretName string
	CreatedAt     string
	UpdatedAt     string
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

type AppEnvVarsRecord struct {
	Name                 string
	AppGUID              string
	SpaceGUID            string
	EnvironmentVariables map[string]string
}

type CurrentDropletRecord struct {
	AppGUID     string
	DropletGUID string
}

func (f *AppRepo) FetchApp(ctx context.Context, client client.Client, appGUID string) (AppRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	appList := &workloadsv1alpha1.CFAppList{}
	err := client.List(ctx, appList)
	if err != nil { // untested
		return AppRecord{}, err
	}
	allApps := appList.Items
	matches := filterAppsByMetadataName(allApps, appGUID)

	return returnApp(matches)
}

func (f *AppRepo) AppExistsWithNameAndSpace(ctx context.Context, c client.Client, appName, spaceGUID string) (bool, error) {
	appList := new(workloadsv1alpha1.CFAppList)
	err := c.List(ctx, appList, client.InNamespace(spaceGUID))
	if err != nil { // untested
		return false, err
	}

	for _, app := range appList.Items {
		if app.Spec.Name == appName {
			return true, nil
		}
	}
	return false, nil
}

func (f *AppRepo) CreateApp(ctx context.Context, client client.Client, appRecord AppRecord) (AppRecord, error) {
	cfApp := appRecordToCFApp(appRecord)
	err := client.Create(ctx, &cfApp)
	if err != nil {
		return AppRecord{}, err
	}
	return cfAppToAppRecord(cfApp), err
}

func (f *AppRepo) FetchAppList(ctx context.Context, client client.Client) ([]AppRecord, error) {
	// TODO: add checks for allowed namespaces with completion of initial auth work
	// Currently we assume the client has full cluster access and naively returns all records
	appList := &workloadsv1alpha1.CFAppList{}
	err := client.List(ctx, appList)
	if err != nil {
		return []AppRecord{}, err
	}
	allApps := appList.Items

	appRecordList := make([]AppRecord, 0, len(allApps))
	for _, app := range allApps {
		appRecordList = append(appRecordList, cfAppToAppRecord(app))
	}

	return appRecordList, nil
}

func (f *AppRepo) FetchNamespace(ctx context.Context, client client.Client, nsGUID string) (SpaceRecord, error) {
	namespace := &v1.Namespace{}
	err := client.Get(ctx, types.NamespacedName{Name: nsGUID}, namespace)
	if err != nil {
		switch errtype := err.(type) {
		case *k8serrors.StatusError:
			reason := errtype.Status().Reason
			if reason == metav1.StatusReasonNotFound || reason == metav1.StatusReasonUnauthorized {
				return SpaceRecord{}, PermissionDeniedOrNotFoundError{Err: err}
			}
		}
		return SpaceRecord{}, err
	}
	return v1NamespaceToSpaceRecord(namespace), nil
}

func (f *AppRepo) CreateAppEnvironmentVariables(ctx context.Context, client client.Client, envVariables AppEnvVarsRecord) (AppEnvVarsRecord, error) {
	secretObj := appEnvVarsRecordToSecret(envVariables)
	err := client.Create(ctx, &secretObj)
	if err != nil {
		return AppEnvVarsRecord{}, err
	}
	return appEnvVarsSecretToRecord(secretObj), nil
}

type SetCurrentDropletMessage struct {
	AppGUID     string
	DropletGUID string
	SpaceGUID   string
}

func (f *AppRepo) SetCurrentDroplet(ctx context.Context, c client.Client, message SetCurrentDropletMessage) (CurrentDropletRecord, error) {
	baseCFApp := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.AppGUID,
			Namespace: message.SpaceGUID,
		},
	}
	cfApp := baseCFApp.DeepCopy()
	cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: message.DropletGUID}

	err := c.Patch(ctx, cfApp, client.MergeFrom(baseCFApp))
	if err != nil {
		return CurrentDropletRecord{}, fmt.Errorf("err in client.Patch: %w", err)
	}

	return CurrentDropletRecord{
		AppGUID:     message.AppGUID,
		DropletGUID: message.DropletGUID,
	}, nil
}

type SetAppDesiredStateMessage struct {
	AppGUID      string
	SpaceGUID    string
	DesiredState string
}

func (f *AppRepo) SetAppDesiredState(ctx context.Context, c client.Client, message SetAppDesiredStateMessage) (AppRecord, error) {
	baseCFApp := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.AppGUID,
			Namespace: message.SpaceGUID,
		},
	}
	cfApp := baseCFApp.DeepCopy()
	cfApp.Spec.DesiredState = workloadsv1alpha1.DesiredState(message.DesiredState)

	err := c.Patch(ctx, cfApp, client.MergeFrom(baseCFApp))
	if err != nil {
		return AppRecord{}, fmt.Errorf("err in client.Patch: %w", err)
	}
	return cfAppToAppRecord(*cfApp), nil
}

func appRecordToCFApp(appRecord AppRecord) workloadsv1alpha1.CFApp {
	return workloadsv1alpha1.CFApp{
		TypeMeta: metav1.TypeMeta{
			Kind:       Kind,
			APIVersion: APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        appRecord.GUID,
			Namespace:   appRecord.SpaceGUID,
			Labels:      appRecord.Labels,
			Annotations: appRecord.Annotations,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:          appRecord.Name,
			DesiredState:  workloadsv1alpha1.DesiredState(appRecord.State),
			EnvSecretName: appRecord.EnvSecretName,
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: workloadsv1alpha1.LifecycleType(appRecord.Lifecycle.Type),
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: appRecord.Lifecycle.Data.Buildpacks,
					Stack:      appRecord.Lifecycle.Data.Stack,
				},
			},
		},
	}
}

func cfAppToAppRecord(cfApp workloadsv1alpha1.CFApp) AppRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfApp.ObjectMeta)

	return AppRecord{
		GUID:        cfApp.Name,
		Name:        cfApp.Spec.Name,
		SpaceGUID:   cfApp.Namespace,
		DropletGUID: cfApp.Spec.CurrentDropletRef.Name,
		Labels:      cfApp.Labels,
		Annotations: cfApp.Annotations,
		State:       DesiredState(cfApp.Spec.DesiredState),
		Lifecycle: Lifecycle{
			Data: LifecycleData{
				Buildpacks: cfApp.Spec.Lifecycle.Data.Buildpacks,
				Stack:      cfApp.Spec.Lifecycle.Data.Stack,
			},
		},
		CreatedAt: cfApp.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt: updatedAtTime,
	}
}

func returnApp(apps []workloadsv1alpha1.CFApp) (AppRecord, error) {
	if len(apps) == 0 {
		return AppRecord{}, NotFoundError{ResourceType: "App"}
	}
	if len(apps) > 1 {
		return AppRecord{}, errors.New("duplicate apps exist")
	}

	return cfAppToAppRecord(apps[0]), nil
}

func filterAppsByMetadataName(apps []workloadsv1alpha1.CFApp, name string) []workloadsv1alpha1.CFApp {
	var filtered []workloadsv1alpha1.CFApp
	for i, app := range apps {
		if app.ObjectMeta.Name == name {
			filtered = append(filtered, apps[i])
		}
	}
	return filtered
}

func v1NamespaceToSpaceRecord(namespace *v1.Namespace) SpaceRecord {
	// TODO How do we derive Organization GUID here?
	return SpaceRecord{
		Name:             namespace.Name,
		OrganizationGUID: "",
	}
}

func appEnvVarsRecordToSecret(envVars AppEnvVarsRecord) corev1.Secret {
	labels := make(map[string]string, 1)
	labels[CFAppGUIDLabel] = envVars.AppGUID
	envSecretName := envVars.AppGUID + "-env"
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envSecretName,
			Namespace: envVars.SpaceGUID,
			Labels:    labels,
		},
		StringData: envVars.EnvironmentVariables,
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
	// StringData is a write-only field of a corev1.Secret, the real data lives in .Data and is []byte & base64 encoded
	marshalledData, _ := json.Marshal(inputMap)
	outputMap := make(map[string]string)
	json.Unmarshal(marshalledData, &outputMap)
	return outputMap
}
