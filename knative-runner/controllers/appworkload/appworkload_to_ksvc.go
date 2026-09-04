package appworkload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

const (
	bindingRootPath = "/bindings"

	EnvCFInstanceIndex    = "CF_INSTANCE_INDEX"
	EnvServiceBindingRoot = "SERVICE_BINDING_ROOT"

	AnnotationVersion     = "korifi.cloudfoundry.org/version"
	AnnotationAppID       = "korifi.cloudfoundry.org/application-id"
	AnnotationProcessGUID = "korifi.cloudfoundry.org/process-guid"

	LabelVersion         = "korifi.cloudfoundry.org/version"
	LabelAppGUID         = "korifi.cloudfoundry.org/app-guid"
	LabelAppWorkloadGUID = "korifi.cloudfoundry.org/appworkload-guid"
	LabelProcessType     = "korifi.cloudfoundry.org/process-type"

	ApplicationContainerName = "user-container"
	ServiceAccountName       = "korifi-app"

	AnnotationMinScale = "autoscaling.knative.dev/min-scale"
	AnnotationMaxScale = "autoscaling.knative.dev/max-scale"
)

// Env vars Knative injects / reserves — must not appear in the container env.
var knativeReservedEnv = map[string]struct{}{
	"PORT":            {},
	"K_REVISION":      {},
	"K_CONFIGURATION": {},
	"K_SERVICE":       {},
}

// Overridable in tests to exercise error paths.
var (
	marshalJSON          = json.Marshal
	unmarshalJSON        = json.Unmarshal
	unstructuredFromJSON = (*unstructured.Unstructured).UnmarshalJSON
)

type AppWorkloadToKnativeServiceConverter struct {
	scheme *runtime.Scheme
}

func NewAppWorkloadToKnativeServiceConverter(scheme *runtime.Scheme) *AppWorkloadToKnativeServiceConverter {
	return &AppWorkloadToKnativeServiceConverter{scheme: scheme}
}

func (r *AppWorkloadToKnativeServiceConverter) Convert(appWorkload *korifiv1alpha1.AppWorkload) (*unstructured.Unstructured, error) {
	labels := korifiLabels(appWorkload)
	specMap, err := marshaledPodSpec(appWorkload)
	if err != nil {
		return nil, err
	}

	return buildKnativeService(
		knativeServiceName(appWorkload),
		appWorkload.Namespace,
		labels,
		serviceAnnotations(appWorkload),
		templateAnnotations(appWorkload),
		specMap,
	)
}

func containerEnv(appWorkload *korifiv1alpha1.AppWorkload) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0, len(appWorkload.Spec.Env)+2)
	for _, env := range appWorkload.Spec.Env {
		if _, reserved := knativeReservedEnv[env.Name]; reserved {
			continue
		}
		// Knative forbids downward-API fieldRefs on the user container.
		if env.ValueFrom != nil && env.ValueFrom.FieldRef != nil {
			continue
		}
		envs = append(envs, env)
	}

	// No StatefulSet ordinal under Knative — CF apps that read the index see "0".
	envs = append(envs, corev1.EnvVar{Name: EnvCFInstanceIndex, Value: "0"})

	if len(appWorkload.Spec.Services) != 0 {
		envs = append(envs, corev1.EnvVar{Name: EnvServiceBindingRoot, Value: bindingRootPath})
	}

	sort.SliceStable(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs
}

func userContainer(appWorkload *korifiv1alpha1.AppWorkload) corev1.Container {
	return corev1.Container{
		Name:            ApplicationContainerName,
		Image:           appWorkload.Spec.Image,
		ImagePullPolicy: corev1.PullAlways,
		Command:         appWorkload.Spec.Command,
		Env:             containerEnv(appWorkload),
		Ports: slices.Collect(it.Map(slices.Values(appWorkload.Spec.Ports), func(port int32) corev1.ContainerPort {
			// Knative allows empty, "h2c", or "http1" only.
			return corev1.ContainerPort{Name: "http1", ContainerPort: port}
		})),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: tools.PtrTo(false),
			RunAsNonRoot:             tools.PtrTo(true),
			// Numeric UID required: kubelet cannot verify named users (e.g. "app")
			// against runAsNonRoot. 1000 is the Paketo/cnb convention Korifi apps use.
			RunAsUser: tools.PtrTo(int64(1000)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Resources:     appWorkload.Spec.Resources,
		StartupProbe:  appWorkload.Spec.StartupProbe,
		LivenessProbe: appWorkload.Spec.LivenessProbe,
		VolumeMounts:  serviceVolumeMounts(appWorkload.Spec.Services),
	}
}

func serviceVolumeMounts(services []korifiv1alpha1.ServiceBinding) []corev1.VolumeMount {
	return slices.Collect(it.Map(slices.Values(services), func(s korifiv1alpha1.ServiceBinding) corev1.VolumeMount {
		return corev1.VolumeMount{
			Name:      s.Name,
			ReadOnly:  true,
			MountPath: filepath.Join(bindingRootPath, s.Name),
		}
	}))
}

func serviceVolumes(services []korifiv1alpha1.ServiceBinding) []corev1.Volume {
	return slices.Collect(it.Map(slices.Values(services), func(s korifiv1alpha1.ServiceBinding) corev1.Volume {
		return corev1.Volume{
			Name: s.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  s.Secret,
					DefaultMode: tools.PtrTo[int32](0o644),
				},
			},
		}
	}))
}

func korifiLabels(appWorkload *korifiv1alpha1.AppWorkload) map[string]string {
	return map[string]string{
		controllers.LabelGUID: appWorkload.Spec.GUID,
		LabelProcessType:      appWorkload.Spec.ProcessType,
		LabelVersion:          appWorkload.Spec.Version,
		LabelAppGUID:          appWorkload.Spec.AppGUID,
		LabelAppWorkloadGUID:  appWorkload.Name,
		// Korifi's log-cache/process stats look up instance index via this
		// StatefulSet-standard label. Knative pods don't get it automatically.
		"apps.kubernetes.io/pod-index": "0",
	}
}

func processGUID(appWorkload *korifiv1alpha1.AppWorkload) string {
	return fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, appWorkload.Spec.Version)
}

func serviceAnnotations(appWorkload *korifiv1alpha1.AppWorkload) map[string]string {
	return map[string]string{
		AnnotationAppID:       appWorkload.Spec.AppGUID,
		AnnotationVersion:     appWorkload.Spec.Version,
		AnnotationProcessGUID: processGUID(appWorkload),
	}
}

func templateAnnotations(appWorkload *korifiv1alpha1.AppWorkload) map[string]string {
	instances := max(appWorkload.Spec.Instances, 0)
	// Desired count: min=max=N. Stopped (N=0): min 0, max 1 so the Service remains.
	return map[string]string{
		AnnotationAppID:       appWorkload.Spec.AppGUID,
		AnnotationVersion:     appWorkload.Spec.Version,
		AnnotationProcessGUID: processGUID(appWorkload),
		AnnotationMinScale:    strconv.FormatInt(int64(instances), 10),
		AnnotationMaxScale:    strconv.FormatInt(int64(max(instances, 1)), 10),
	}
}

func podSpec(appWorkload *korifiv1alpha1.AppWorkload) corev1.PodSpec {
	return corev1.PodSpec{
		Containers:                   []corev1.Container{userContainer(appWorkload)},
		ImagePullSecrets:             appWorkload.Spec.ImagePullSecrets,
		ServiceAccountName:           ServiceAccountName,
		AutomountServiceAccountToken: tools.PtrTo(false),
		// Knative forbids setting pod-level securityContext here.
		Volumes: serviceVolumes(appWorkload.Spec.Services),
	}
}

func marshaledPodSpec(appWorkload *korifiv1alpha1.AppWorkload) (map[string]any, error) {
	bytes, err := marshalJSON(podSpec(appWorkload))
	if err != nil {
		return nil, fmt.Errorf("marshal pod spec: %w", err)
	}
	var out map[string]any
	if err = unmarshalJSON(bytes, &out); err != nil {
		return nil, fmt.Errorf("unmarshal pod spec: %w", err)
	}
	// Drop null securityContext if present.
	delete(out, "securityContext")
	return out, nil
}

func buildKnativeService(
	name, namespace string,
	labels, serviceAnns, templateAnns map[string]string,
	spec map[string]any,
) (*unstructured.Unstructured, error) {
	raw, err := marshalJSON(map[string]any{
		"apiVersion": "serving.knative.dev/v1",
		"kind":       "Service",
		"metadata": metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: serviceAnns,
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"labels":      labels,
					"annotations": templateAnns,
				},
				"spec": spec,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal knative service: %w", err)
	}

	out := &unstructured.Unstructured{}
	if err := unstructuredFromJSON(out, raw); err != nil {
		return nil, fmt.Errorf("unmarshal knative service: %w", err)
	}
	return out, nil
}

func knativeServiceName(appWorkload *korifiv1alpha1.AppWorkload) string {
	lastStopAppRev := appWorkload.Spec.Version
	if annotationVal, ok := appWorkload.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]; ok {
		lastStopAppRev = annotationVal
	}
	nameSuffix := hash(fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, lastStopAppRev))

	// DNS-1035: must start with a letter. GUID-based prefixes often start with a digit.
	namePrefix := sanitizeName("kw-"+appWorkload.Spec.AppGUID, "kw-"+appWorkload.Spec.GUID)
	return fmt.Sprintf("%s-%s", namePrefix, nameSuffix)
}

func sanitizeName(name, fallback string) string {
	const sanitizedNameMaxLen = 40
	validNameRegex := regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), "_", "-")
	if validNameRegex.MatchString(sanitizedName) {
		return truncateString(sanitizedName, sanitizedNameMaxLen)
	}
	return truncateString(fallback, sanitizedNameMaxLen)
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num]
	}
	return str
}

func hash(s string) string {
	const MaxHashLength = 10
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:MaxHashLength]
}
