package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
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

type SpaceRecord struct {
	Name             string
	OrganizationGUID string
}

type AppEnvVarsRecord struct {
	Name                 string
	AppGUID              string
	SpaceGUID            string
	EnvironmentVariables map[string]string
}

func (f *AppRepo) FetchApp(client client.Client, ctx context.Context, appGUID string) (AppRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	appList := &workloadsv1alpha1.CFAppList{}
	err := client.List(ctx, appList)
	if err != nil {
		return AppRecord{}, err
	}
	allApps := appList.Items
	matches := f.filterAppsByName(allApps, appGUID)

	return f.returnApps(matches)
}

func (f *AppRepo) getAppCR(client client.Client, ctx context.Context, appGUID string, namespace string) (*workloadsv1alpha1.CFApp, error) {
	app := &workloadsv1alpha1.CFApp{}
	err := client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      appGUID,
	}, app)
	return app, err
}

func (f *AppRepo) AppExists(client client.Client, ctx context.Context, appGUID string, namespace string) (bool, error) {
	_, err := f.getAppCR(client, ctx, appGUID, namespace)
	if err != nil {
		switch errtype := err.(type) {
		case *k8serrors.StatusError:
			reason := errtype.Status().Reason
			if reason == metav1.StatusReasonNotFound {
				return false, nil
			}
		default:
			return true, err
		}
	}
	return true, nil
}

func (f *AppRepo) CreateApp(client client.Client, ctx context.Context, appRecord AppRecord) (AppRecord, error) {
	cfApp := f.appRecordToCFApp(appRecord)
	err := client.Create(ctx, &cfApp)
	if err != nil {
		return AppRecord{}, err
	}
	return f.cfAppToResponseApp(cfApp), err
}

func (f *AppRepo) appRecordToCFApp(appRecord AppRecord) workloadsv1alpha1.CFApp {
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

func (f *AppRepo) cfAppToResponseApp(cfApp workloadsv1alpha1.CFApp) AppRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfApp.ObjectMeta)

	return AppRecord{
		GUID:        cfApp.Name,
		Name:        cfApp.Spec.Name,
		SpaceGUID:   cfApp.Namespace,
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

func (f *AppRepo) returnApps(apps []workloadsv1alpha1.CFApp) (AppRecord, error) {
	if len(apps) == 0 {
		return AppRecord{}, NotFoundError{Err: errors.New("not found")}
	}
	if len(apps) > 1 {
		return AppRecord{}, errors.New("duplicate apps exist")
	}

	return f.cfAppToResponseApp(apps[0]), nil
}

func (f *AppRepo) filterAppsByName(apps []workloadsv1alpha1.CFApp, name string) []workloadsv1alpha1.CFApp {
	filtered := []workloadsv1alpha1.CFApp{}
	for i, app := range apps {
		if app.Name == name {
			filtered = append(filtered, apps[i])
		}
	}
	return filtered
}

func (f *AppRepo) FetchNamespace(client client.Client, ctx context.Context, nsGUID string) (SpaceRecord, error) {
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
	return f.v1NamespaceToSpaceRecord(namespace), nil
}

func (f *AppRepo) v1NamespaceToSpaceRecord(namespace *v1.Namespace) SpaceRecord {
	//TODO How do we derive Organization GUID here?
	return SpaceRecord{
		Name:             namespace.Name,
		OrganizationGUID: "",
	}
}

func (f *AppRepo) CreateAppEnvironmentVariables(client client.Client, ctx context.Context, envVariables AppEnvVarsRecord) (AppEnvVarsRecord, error) {
	secretObj := f.appEnvVarsRecordToSecret(envVariables)
	err := client.Create(ctx, &secretObj)
	if err != nil {
		return AppEnvVarsRecord{}, err
	}
	return f.appEnvVarsSecretToRecord(secretObj), nil
}

var staticCFApp workloadsv1alpha1.CFApp

func (f *AppRepo) GenerateEnvSecretName(appGUID string) string {
	return appGUID + "-env"
}
func (f *AppRepo) extractAppGUIDFromEnvSecretName(envSecretName string) string {
	return strings.Trim(envSecretName, "-env")
}

func (f *AppRepo) appEnvVarsRecordToSecret(envVars AppEnvVarsRecord) corev1.Secret {
	labels := make(map[string]string, 1)
	labels[CFAppGUIDLabel] = envVars.AppGUID
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.GenerateEnvSecretName(envVars.AppGUID),
			Namespace: envVars.SpaceGUID,
			Labels:    labels,
		},
		StringData: envVars.EnvironmentVariables,
	}
}

func (f *AppRepo) appEnvVarsSecretToRecord(envVars corev1.Secret) AppEnvVarsRecord {
	return AppEnvVarsRecord{
		Name:      envVars.Name,
		AppGUID:   f.extractAppGUIDFromEnvSecretName(envVars.Name),
		SpaceGUID: envVars.Namespace,
		// StringData is a write-only field of a corev1.Secret, the real data lives in .Data and is []byte & base64 encoded
		EnvironmentVariables: convertMapStringByteToMapStringString(envVars.Data),
	}
}

func convertMapStringByteToMapStringString(inputMap map[string][]byte) map[string]string {
	marshalledData, _ := json.Marshal(inputMap)
	outputMap := make(map[string]string)
	json.Unmarshal(marshalledData, &outputMap)
	return outputMap
}
