package repositories

import (
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"k8s.io/apimachinery/pkg/api/meta"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

func getLabelOrAnnotation(mapObj map[string]string, key string) string {
	if mapObj == nil {
		return ""
	}
	return mapObj[key]
}

func wrapK8sErr(err error, resourceType string) error {
	switch {
	case k8serrors.IsNotFound(err):
		return NewNotFoundError(resourceType, err)
	case k8serrors.IsForbidden(err):
		return NewForbiddenError(resourceType, err)
	case k8serrors.IsUnauthorized(err):
		return authorization.InvalidAuthError{Err: err}
	default:
		return err
	}
}
