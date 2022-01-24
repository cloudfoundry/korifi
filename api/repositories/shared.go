package repositories

import (
	"errors"

	rbacv1 "k8s.io/api/rbac/v1"
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

func getLabelOrAnnotation(mapObj map[string]string, key string) string {
	if mapObj == nil {
		return ""
	}
	return mapObj[key]
}

//  common permissions shared between multiple test packages
var (
	AdminClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"create", "delete"},
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"rolebindings"},
		},
		{
			Verbs:     []string{"create"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"subnamespaceanchors"},
		},
	}

	SpaceDeveloperClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "patch"},
			APIGroups: []string{""},
			Resources: []string{"secrets"},
		},
		{
			Verbs:     []string{"get", "list", "create", "patch", "delete"},
			APIGroups: []string{"workloads.cloudfoundry.org"},
			Resources: []string{"cfapps"},
		},
		{
			Verbs:     []string{"get", "list", "create", "delete"},
			APIGroups: []string{"networking.cloudfoundry.org"},
			Resources: []string{"cfroutes"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"kpack.io"},
			Resources: []string{"clusterbuilders"},
		},
	}

	SpaceManagerClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"workloads.cloudfoundry.org"},
			Resources: []string{"cfapps"},
		},
	}

	SpaceAuditorClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{"workloads.cloudfoundry.org"},
			Resources: []string{"cfapps"},
		},
		{
			Verbs:     []string{"get"},
			APIGroups: []string{"kpack.io"},
			Resources: []string{"clusterbuilders"},
		},
	}

	OrgManagerClusterRoleRules = []rbacv1.PolicyRule{
		{
			Verbs:     []string{"list", "delete"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"subnamespaceanchors"},
		},
		{
			Verbs:     []string{"get", "update"},
			APIGroups: []string{"hnc.x-k8s.io"},
			Resources: []string{"hierarchyconfigurations"},
		},
	}

	OrgUserClusterRoleRules = []rbacv1.PolicyRule{}
)
