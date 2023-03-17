package repositories

import (
	"context"
	"errors"
	"regexp"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StatusConditionReady                 = "Ready"
	VCAPServicesSecretAvailableCondition = "VCAPServicesSecretAvailable"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator
type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

type ConditionAwaiter[T runtime.Object] interface {
	AwaitCondition(ctx context.Context, userClient client.WithWatch, object client.Object, conditionType string) (T, error)
}

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

func matchesFilter(field string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}

	for _, value := range filter {
		if field == value {
			return true
		}
	}

	return false
}

func labelsFilters(labels map[string]string, query []string) bool {
	if len(query) == 0 {
		return true
	}

	for _, value := range query {
		if !labelsFilter(labels, value) {
			return false
		}
	}

	return true
}

func labelsFilter(labels map[string]string, query string) bool {
	// TODO: This is a simple query tool and needs to be enhanced
	re := regexp.MustCompile(`(^[a-zA-Z0-9./-_]+)(!{0,1}={1,2})([a-zA-Z0-9./-_]+)$`)

	matches := re.FindStringSubmatch(query)
	if matches != nil {
		k := matches[1]
		op := matches[2]
		v := matches[3]

		if value, present := labels[k]; present {
			switch op {
			case "=":
				fallthrough
			case "==":
				return value == v
			case "!=":
				return value != v
			}
		}

		return false
	}

	if query[0] == '!' {
		if _, present := labels[query[1:]]; !present {
			return true
		}
	} else {
		if _, present := labels[query]; present {
			return true
		}
	}
	return false
}
