package repositories

import (
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// getTimeLastUpdatedTimestamp takes the ObjectMeta from a CR and extracts the last updated time from its list of ManagedFields
// Returns an error if the list is empty or the time could not be extracted
func getTimeLastUpdatedTimestamp(metadata *metav1.ObjectMeta) (string, error) {
	if len(metadata.ManagedFields) == 0 {
		return "", errors.New("error, metadata.ManagedFields was empty")
	}

	latestTime := metadata.ManagedFields[0].Time
	for i := 1; i < len(metadata.ManagedFields); i++ {
		currentTime := metadata.ManagedFields[i].Time
		if latestTime == nil {
			latestTime = currentTime
		} else if currentTime != nil {
			if currentTime.After(latestTime.Time) {
				latestTime = currentTime
			}
		}
	}

	if latestTime == nil {
		return "", errors.New("error, could not find a time in metadata.ManagedFields")
	}

	return formatTimestamp(*latestTime), nil
}

func formatTimestamp(time metav1.Time) string {
	return time.UTC().Format(TimestampFormat)
}

// getConditionValue is a helper function that retrieves the value of the provided conditionType, like "Succeeded" and returns the value: "True", "False", or "Unknown"
// If the value is not present, returns Unknown
func getConditionValue(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	conditionStatusValue := metav1.ConditionUnknown
	conditionStatus := meta.FindStatusCondition(*conditions, conditionType)
	if conditionStatus != nil {
		conditionStatusValue = conditionStatus.Status
	}
	return conditionStatusValue
}

type NotFoundError struct {
	Err          error
	ResourceType string
}

func (e NotFoundError) Error() string {
	return "not found"
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}

type PermissionDeniedOrNotFoundError struct {
	Err error
}

func (e PermissionDeniedOrNotFoundError) Error() string {
	return "Resource not found or permission denied."
}

func (e PermissionDeniedOrNotFoundError) Unwrap() error {
	return e.Err
}

type ResourceNotFoundError struct {
	Err error
}

func (e ResourceNotFoundError) Error() string {
	return "Resource not found."
}

func (e ResourceNotFoundError) Unwrap() error {
	return e.Err
}
