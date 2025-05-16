package rules

import (
	"fmt"
	"strconv"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
	"github.com/PaesslerAG/jsonpath"
)

func BuildStateLabelRules(obj map[string]any) ([]LabelRule, error) {
	buildState, err := computeBuildState(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to compute build state: %w", err)
	}

	return []LabelRule{{Label: korifiv1alpha1.BuildStateLabelKey, IndexingFunc: values.ConstantValue(buildState)}}, nil
}

func computeBuildState(obj map[string]any) (string, error) {
	isSucceeded, err := getConditionStatus(obj, korifiv1alpha1.SucceededConditionType)
	if err != nil {
		return "", fmt.Errorf("failed to get succeeded condition status: %w", err)
	}

	isStaging, err := getConditionStatus(obj, korifiv1alpha1.StagingConditionType)
	if err != nil {
		return "", fmt.Errorf("failed to get staging condition status: %w", err)
	}

	if isSucceeded {
		return korifiv1alpha1.BuildStateStaged, nil
	}

	if isStaging {
		return korifiv1alpha1.BuildStateStaging, nil
	}

	return korifiv1alpha1.BuildStateFailed, nil
}

func getConditionStatus(obj map[string]any, conditionType string) (bool, error) {
	conditionStatus, err := jsonpath.Get(fmt.Sprintf("{$.status.conditions[?(@.type=='%s')].status}", conditionType), obj)
	if err != nil {
		return false, fmt.Errorf("failed to get status for condition %s: %w", conditionType, err)
	}

	if conditionStatus == "" {
		return false, nil
	}

	return strconv.ParseBool(conditionStatus.(string))
}
