package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ProcessResourceType = "Process"

func NewProcessRepo(klient Klient) *ProcessRepo {
	return &ProcessRepo{
		klient: klient,
	}
}

type ProcessRepo struct {
	klient Klient
}

type ProcessRecord struct {
	GUID             string
	SpaceGUID        string
	AppGUID          string
	Type             string
	Command          string
	DesiredInstances int32
	MemoryMB         int64
	DiskQuotaMB      int64
	HealthCheck      HealthCheck
	Labels           map[string]string
	Annotations      map[string]string
	CreatedAt        time.Time
	UpdatedAt        *time.Time
	InstancesStatus  map[string]korifiv1alpha1.InstanceStatus
}

func (r ProcessRecord) Relationships() map[string]string {
	return map[string]string{
		"app": r.AppGUID,
	}
}

func (r ProcessRecord) GetResourceType() string {
	return ProcessResourceType
}

type HealthCheck struct {
	Type string
	Data HealthCheckData
}

type HealthCheckData struct {
	HTTPEndpoint             string
	InvocationTimeoutSeconds int32
	TimeoutSeconds           int32
}

type ScaleProcessMessage struct {
	GUID      string
	SpaceGUID string
	ProcessScaleValues
}

type ProcessScaleValues struct {
	Instances *int32
	MemoryMB  *int64
	DiskMB    *int64
}

type CreateProcessMessage struct {
	AppGUID          string
	SpaceGUID        string
	Type             string
	Command          string
	DiskQuotaMB      int64
	HealthCheck      HealthCheck
	DesiredInstances *int32
	MemoryMB         int64
}

type PatchProcessMessage struct {
	SpaceGUID                           string
	ProcessGUID                         string
	Command                             *string
	DiskQuotaMB                         *int64
	HealthCheckHTTPEndpoint             *string
	HealthCheckInvocationTimeoutSeconds *int32
	HealthCheckTimeoutSeconds           *int32
	HealthCheckType                     *string
	DesiredInstances                    *int32
	MemoryMB                            *int64
	MetadataPatch                       *MetadataPatch
}

type ListProcessesMessage struct {
	AppGUIDs     []string
	ProcessTypes []string
	SpaceGUIDs   []string
	OrderBy      string
	Pagination   Pagination
}

func (m *ListProcessesMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, m.AppGUIDs),
		WithLabelIn(korifiv1alpha1.CFProcessTypeLabelKey, m.ProcessTypes),
		WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, m.SpaceGUIDs),
		WithOrdering(m.OrderBy),
		WithPaging(m.Pagination),
	}
}

func (r *ProcessRepo) GetProcess(ctx context.Context, authInfo authorization.Info, processGUID string) (ProcessRecord, error) {
	process := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name: processGUID,
		},
	}
	err := r.klient.Get(ctx, process)
	if err != nil {
		return ProcessRecord{}, fmt.Errorf("failed to get process %q: %w", processGUID, apierrors.FromK8sError(err, ProcessResourceType))
	}

	return cfProcessToProcessRecord(*process)
}

func (r *ProcessRepo) ListProcesses(ctx context.Context, authInfo authorization.Info, message ListProcessesMessage) (ListResult[ProcessRecord], error) {
	processList := &korifiv1alpha1.CFProcessList{}
	pageInfo, err := r.klient.List(ctx, processList, message.toListOptions()...)
	if err != nil {
		return ListResult[ProcessRecord]{}, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	records, err := it.TryCollect(it.MapError(itx.FromSlice(processList.Items), cfProcessToProcessRecord))
	if err != nil {
		return ListResult[ProcessRecord]{}, fmt.Errorf("failed to convert processes to records: %w", err)
	}

	return ListResult[ProcessRecord]{
		PageInfo: pageInfo,
		Records:  records,
	}, nil
}

func (r *ProcessRepo) ScaleProcess(ctx context.Context, authInfo authorization.Info, scaleProcessMessage ScaleProcessMessage) (ProcessRecord, error) {
	cfProcess := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scaleProcessMessage.GUID,
			Namespace: scaleProcessMessage.SpaceGUID,
		},
	}
	err := GetAndPatch(ctx, r.klient, cfProcess, func() error {
		if scaleProcessMessage.Instances != nil {
			cfProcess.Spec.DesiredInstances = scaleProcessMessage.Instances
		}
		if scaleProcessMessage.MemoryMB != nil {
			cfProcess.Spec.MemoryMB = *scaleProcessMessage.MemoryMB
		}
		if scaleProcessMessage.DiskMB != nil {
			cfProcess.Spec.DiskQuotaMB = *scaleProcessMessage.DiskMB
		}

		return nil
	})
	if err != nil {
		return ProcessRecord{}, fmt.Errorf("failed to scale process %q: %w", scaleProcessMessage.GUID, apierrors.FromK8sError(err, ProcessResourceType))
	}

	return cfProcessToProcessRecord(*cfProcess)
}

func (r *ProcessRepo) CreateProcess(ctx context.Context, authInfo authorization.Info, message CreateProcessMessage) error {
	process := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      tools.NamespacedUUID(message.AppGUID, message.Type),
		},
		Spec: korifiv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: message.AppGUID},
			ProcessType: message.Type,
			Command:     message.Command,
			HealthCheck: korifiv1alpha1.HealthCheck{
				Type: korifiv1alpha1.HealthCheckType(message.HealthCheck.Type),
				Data: korifiv1alpha1.HealthCheckData(message.HealthCheck.Data),
			},
			DesiredInstances: message.DesiredInstances,
			MemoryMB:         message.MemoryMB,
			DiskQuotaMB:      message.DiskQuotaMB,
		},
	}
	err := r.klient.Create(ctx, process)
	return apierrors.FromK8sError(err, ProcessResourceType)
}

func (r *ProcessRepo) GetAppRevision(ctx context.Context, authInfo authorization.Info, appGUID string) (string, error) {
	app := korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name: appGUID,
		},
	}
	err := r.klient.Get(ctx, &app)
	if err != nil {
		return "", fmt.Errorf("get-apprevision-for-process: failed to get app from kubernetes: %w", apierrors.FromK8sError(err, ProcessResourceType))
	}

	appRevision := app.ObjectMeta.Annotations["korifi.cloudfoundry.org/app-rev"]
	if appRevision == "" {
		return appRevision, fmt.Errorf("get-apprevision-for-process: cannot find app revision")
	}

	return appRevision, nil
}

func (r *ProcessRepo) PatchProcess(ctx context.Context, authInfo authorization.Info, message PatchProcessMessage) (ProcessRecord, error) {
	updatedProcess := &korifiv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.ProcessGUID,
			Namespace: message.SpaceGUID,
		},
	}
	err := GetAndPatch(ctx, r.klient, updatedProcess, func() error {
		if message.Command != nil {
			updatedProcess.Spec.Command = *message.Command
		}
		if message.DesiredInstances != nil {
			updatedProcess.Spec.DesiredInstances = message.DesiredInstances
		}
		if message.MemoryMB != nil {
			updatedProcess.Spec.MemoryMB = *message.MemoryMB
		}
		if message.DiskQuotaMB != nil {
			updatedProcess.Spec.DiskQuotaMB = *message.DiskQuotaMB
		}
		if message.HealthCheckType != nil {
			// TODO: how do we handle when the type changes? Clear the HTTPEndpoint when type != http? Should we require the endpoint when type == http?
			updatedProcess.Spec.HealthCheck.Type = korifiv1alpha1.HealthCheckType(*message.HealthCheckType)
		}
		if message.HealthCheckHTTPEndpoint != nil {
			updatedProcess.Spec.HealthCheck.Data.HTTPEndpoint = *message.HealthCheckHTTPEndpoint
		}
		if message.HealthCheckInvocationTimeoutSeconds != nil {
			updatedProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds = *message.HealthCheckInvocationTimeoutSeconds
		}
		if message.HealthCheckTimeoutSeconds != nil {
			updatedProcess.Spec.HealthCheck.Data.TimeoutSeconds = *message.HealthCheckTimeoutSeconds
		}
		if message.MetadataPatch != nil {
			message.MetadataPatch.Apply(updatedProcess)
		}

		return nil
	})
	if err != nil {
		return ProcessRecord{}, apierrors.FromK8sError(err, ProcessResourceType)
	}

	return cfProcessToProcessRecord(*updatedProcess)
}

func cfProcessToProcessRecord(cfProcess korifiv1alpha1.CFProcess) (ProcessRecord, error) {
	createdAt, updatedAt, err := getCreatedUpdatedAt(&cfProcess)
	if err != nil {
		return ProcessRecord{}, fmt.Errorf("failed to parse timestamps for process %q: %w", cfProcess.Name, err)
	}

	cmd := cfProcess.Spec.Command
	if cmd == "" {
		cmd = cfProcess.Spec.DetectedCommand
	}

	return ProcessRecord{
		GUID:             cfProcess.Name,
		SpaceGUID:        cfProcess.Namespace,
		AppGUID:          cfProcess.Spec.AppRef.Name,
		Type:             cfProcess.Spec.ProcessType,
		Command:          cmd,
		DesiredInstances: tools.ZeroIfNil(cfProcess.Spec.DesiredInstances),
		MemoryMB:         cfProcess.Spec.MemoryMB,
		DiskQuotaMB:      cfProcess.Spec.DiskQuotaMB,
		HealthCheck: HealthCheck{
			Type: string(cfProcess.Spec.HealthCheck.Type),
			Data: HealthCheckData{
				HTTPEndpoint:             cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
				InvocationTimeoutSeconds: cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds,
				TimeoutSeconds:           cfProcess.Spec.HealthCheck.Data.TimeoutSeconds,
			},
		},
		Labels:          cfProcess.Labels,
		Annotations:     cfProcess.Annotations,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		InstancesStatus: cfProcess.Status.InstancesStatus,
	}, nil
}
