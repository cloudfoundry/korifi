package jobs

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

const (
	TaskSourceType = "TASK"

	AnnotationGUID                        = "korifi.cloudfoundry.org/guid"
	AnnotationAppName                     = stset.AnnotationAppName
	AnnotationAppID                       = stset.AnnotationAppID
	AnnotationOrgName                     = stset.AnnotationOrgName
	AnnotationOrgGUID                     = stset.AnnotationOrgGUID
	AnnotationSpaceName                   = stset.AnnotationSpaceName
	AnnotationSpaceGUID                   = stset.AnnotationSpaceGUID
	AnnotationTaskContainerName           = "korifi.cloudfoundry.org/opi-task-container-name"
	AnnotationTaskCompletionReportCounter = "korifi.cloudfoundry.org/task_completion_report_counter"
	AnnotationCCAckedTaskCompletion       = "korifi.cloudfoundry.org/cc_acked_task_completion"

	LabelGUID          = stset.LabelGUID
	LabelName          = "korifi.cloudfoundry.org/name"
	LabelAppGUID       = stset.LabelAppGUID
	LabelSourceType    = stset.LabelSourceType
	LabelTaskCompleted = "korifi.cloudfoundry.org/task_completed"

	TaskCompletedTrue                 = "true"
	PrivateRegistrySecretGenerateName = stset.PrivateRegistrySecretGenerateName
)
