package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfprocesses/status,verbs=get

func NewProcessRepo(privilegedClient client.Client) *ProcessRepo {
	return &ProcessRepo{privilegedClient: privilegedClient}
}

type ProcessRepo struct {
	privilegedClient client.Client
}

type ProcessRecord struct {
	GUID             string
	SpaceGUID        string
	AppGUID          string
	Type             string
	Command          string
	DesiredInstances int
	MemoryMB         int64
	DiskQuotaMB      int64
	Ports            []int32
	HealthCheck      HealthCheck
	Labels           map[string]string
	Annotations      map[string]string
	CreatedAt        string
	UpdatedAt        string
}

type HealthCheck struct {
	Type string
	Data HealthCheckData
}

type HealthCheckData struct {
	HTTPEndpoint             string
	InvocationTimeoutSeconds int64
	TimeoutSeconds           int64
}

type ScaleProcessMessage struct {
	GUID      string
	SpaceGUID string
	ProcessScaleValues
}

type ProcessScaleValues struct {
	Instances *int
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
	DesiredInstances int
	MemoryMB         int64
}

type PatchProcessMessage struct {
	SpaceGUID                           string
	ProcessGUID                         string
	Command                             *string
	DiskQuotaMB                         *int64
	HealthCheckHTTPEndpoint             *string
	HealthCheckInvocationTimeoutSeconds *int64
	HealthCheckTimeoutSeconds           *int64
	HealthCheckType                     *string
	DesiredInstances                    *int
	MemoryMB                            *int64
}

type ListProcessesMessage struct {
	AppGUID   []string
	SpaceGUID string
}

func (r *ProcessRepo) GetProcess(ctx context.Context, authInfo authorization.Info, processGUID string) (ProcessRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	processList := &workloadsv1alpha1.CFProcessList{}
	err := r.privilegedClient.List(ctx, processList, client.MatchingFields{"metadata.name": processGUID})
	if err != nil { // untested
		return ProcessRecord{}, err
	}
	return returnProcess(processList.Items)
}

func (r *ProcessRepo) ListProcesses(ctx context.Context, authInfo authorization.Info, message ListProcessesMessage) ([]ProcessRecord, error) {
	processList := &workloadsv1alpha1.CFProcessList{}
	var options []client.ListOption
	if message.SpaceGUID != "" {
		options = []client.ListOption{
			client.InNamespace(message.SpaceGUID),
		}
	}
	err := r.privilegedClient.List(ctx, processList, options...)
	if err != nil { // untested
		return []ProcessRecord{}, err
	}
	allProcesses := processList.Items
	matches := filterProcessesByAppGUID(allProcesses, message.AppGUID)

	return returnProcesses(matches)
}

func (r *ProcessRepo) ScaleProcess(ctx context.Context, authInfo authorization.Info, scaleProcessMessage ScaleProcessMessage) (ProcessRecord, error) {
	baseCFProcess := &workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scaleProcessMessage.GUID,
			Namespace: scaleProcessMessage.SpaceGUID,
		},
	}
	cfProcess := baseCFProcess.DeepCopy()
	if scaleProcessMessage.Instances != nil {
		cfProcess.Spec.DesiredInstances = *scaleProcessMessage.Instances
	}
	if scaleProcessMessage.MemoryMB != nil {
		cfProcess.Spec.MemoryMB = *scaleProcessMessage.MemoryMB
	}
	if scaleProcessMessage.DiskMB != nil {
		cfProcess.Spec.DiskQuotaMB = *scaleProcessMessage.DiskMB
	}

	err := r.privilegedClient.Patch(ctx, cfProcess, client.MergeFrom(baseCFProcess))
	if err != nil {
		return ProcessRecord{}, fmt.Errorf("err in client.Patch: %w", err)
	}

	record := cfProcessToProcessRecord(*cfProcess)
	return record, nil
}

func (r *ProcessRepo) CreateProcess(ctx context.Context, authInfo authorization.Info, message CreateProcessMessage) error {
	guid := uuid.NewString()
	err := r.privilegedClient.Create(ctx, &workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: message.SpaceGUID,
		},
		Spec: workloadsv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: message.AppGUID},
			ProcessType: message.Type,
			Command:     message.Command,
			HealthCheck: workloadsv1alpha1.HealthCheck{
				Type: workloadsv1alpha1.HealthCheckType(message.HealthCheck.Type),
				Data: workloadsv1alpha1.HealthCheckData(message.HealthCheck.Data),
			},
			DesiredInstances: message.DesiredInstances,
			MemoryMB:         message.MemoryMB,
			DiskQuotaMB:      message.DiskQuotaMB,
			Ports:            []int32{},
		},
	})
	return err
}

func (r *ProcessRepo) GetProcessByAppTypeAndSpace(ctx context.Context, authInfo authorization.Info, appGUID, processType, spaceGUID string) (ProcessRecord, error) {
	// Could narrow down process results via AppGUID label, but that is set up by a webhook that isn't configured in our integration tests
	// For now, don't use labels
	var processList workloadsv1alpha1.CFProcessList
	err := r.privilegedClient.List(ctx, &processList, client.InNamespace(spaceGUID))
	if err != nil {
		return ProcessRecord{}, err
	}

	var matches []workloadsv1alpha1.CFProcess
	for _, process := range processList.Items {
		if process.Spec.AppRef.Name == appGUID && process.Spec.ProcessType == processType {
			matches = append(matches, process)
		}
	}
	return returnProcess(matches)
}

func (r *ProcessRepo) PatchProcess(ctx context.Context, authInfo authorization.Info, message PatchProcessMessage) error {
	baseProcess := &workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.ProcessGUID,
			Namespace: message.SpaceGUID,
		},
	}
	updatedProcess := baseProcess.DeepCopy()
	if message.Command != nil {
		updatedProcess.Spec.Command = *message.Command
	}
	if message.DesiredInstances != nil {
		updatedProcess.Spec.DesiredInstances = *message.DesiredInstances
	}
	if message.MemoryMB != nil {
		updatedProcess.Spec.MemoryMB = *message.MemoryMB
	}
	if message.DiskQuotaMB != nil {
		updatedProcess.Spec.DiskQuotaMB = *message.DiskQuotaMB
	}
	if message.HealthCheckType != nil {
		// TODO: how do we handle when the type changes? Clear the HTTPEndpoint when type != http? Should we require the endpoint when type == http?
		updatedProcess.Spec.HealthCheck.Type = workloadsv1alpha1.HealthCheckType(*message.HealthCheckType)
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

	err := r.privilegedClient.Patch(ctx, updatedProcess, client.MergeFrom(baseProcess))
	return err
}

func returnProcess(processes []workloadsv1alpha1.CFProcess) (ProcessRecord, error) {
	if len(processes) == 0 {
		return ProcessRecord{}, NotFoundError{ResourceType: "Process"}
	}
	if len(processes) > 1 {
		return ProcessRecord{}, errors.New("duplicate processes exist")
	}

	return cfProcessToProcessRecord(processes[0]), nil
}

func filterProcessesByAppGUID(processes []workloadsv1alpha1.CFProcess, appGUIDs []string) []workloadsv1alpha1.CFProcess {
	if len(appGUIDs) == 0 {
		return processes
	}

	var filtered []workloadsv1alpha1.CFProcess
	for _, process := range processes {
		for _, appGUID := range appGUIDs {
			if process.Spec.AppRef.Name == appGUID {
				filtered = append(filtered, process)
				break
			}
		}
	}
	return filtered
}

func returnProcesses(processes []workloadsv1alpha1.CFProcess) ([]ProcessRecord, error) {
	processRecords := make([]ProcessRecord, 0, len(processes))
	for _, process := range processes {
		processRecord := cfProcessToProcessRecord(process)
		processRecords = append(processRecords, processRecord)
	}

	return processRecords, nil
}

func cfProcessToProcessRecord(cfProcess workloadsv1alpha1.CFProcess) ProcessRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfProcess.ObjectMeta)

	return ProcessRecord{
		GUID:             cfProcess.Name,
		SpaceGUID:        cfProcess.Namespace,
		AppGUID:          cfProcess.Spec.AppRef.Name,
		Type:             cfProcess.Spec.ProcessType,
		Command:          cfProcess.Spec.Command,
		DesiredInstances: cfProcess.Spec.DesiredInstances,
		MemoryMB:         cfProcess.Spec.MemoryMB,
		DiskQuotaMB:      cfProcess.Spec.DiskQuotaMB,
		Ports:            cfProcess.Spec.Ports,
		HealthCheck: HealthCheck{
			Type: string(cfProcess.Spec.HealthCheck.Type),
			Data: HealthCheckData{
				HTTPEndpoint:             cfProcess.Spec.HealthCheck.Data.HTTPEndpoint,
				InvocationTimeoutSeconds: cfProcess.Spec.HealthCheck.Data.InvocationTimeoutSeconds,
				TimeoutSeconds:           cfProcess.Spec.HealthCheck.Data.TimeoutSeconds,
			},
		},
		Labels:      map[string]string{},
		Annotations: map[string]string{},
		CreatedAt:   cfProcess.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:   updatedAtTime,
	}
}
