package utils

import (
	"fmt"
	"regexp"
	"strings"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"github.com/pkg/errors"
)

const (
	sanitizedNameMaxLen    = 40
	sanitizedJobNameMaxLen = 50
)

func sanitizeName(name, fallback string) string {
	return sanitizeNameWithMaxStringLen(name, fallback, sanitizedNameMaxLen)
}

func sanitizeNameWithMaxStringLen(name, fallback string, maxStringLen int) string {
	validNameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), "_", "-")

	if validNameRegex.MatchString(sanitizedName) {
		return truncateString(sanitizedName, maxStringLen)
	}

	return truncateString(fallback, maxStringLen)
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num]
	}

	return str
}

func GetStatefulsetName(lrp *eiriniv1.LRP) (string, error) {
	nameSuffix, err := util.Hash(fmt.Sprintf("%s-%s", lrp.Spec.GUID, lrp.Spec.Version))
	if err != nil {
		return "", errors.Wrap(err, "failed to generate hash")
	}

	namePrefix := fmt.Sprintf("%s-%s", lrp.Spec.AppName, lrp.Spec.SpaceName)
	namePrefix = sanitizeName(namePrefix, lrp.Spec.GUID)

	return fmt.Sprintf("%s-%s", namePrefix, nameSuffix), nil
}

func GetJobName(task *eiriniv1.Task) string {
	name := fmt.Sprintf("%s-%s", task.Spec.AppName, task.Spec.SpaceName)
	sanitizedName := sanitizeName(name, task.Spec.GUID)

	if task.Spec.Name != "" {
		sanitizedName = fmt.Sprintf("%s-%s", sanitizedName, task.Spec.Name)
	}

	return sanitizeNameWithMaxStringLen(sanitizedName, task.Spec.GUID, sanitizedJobNameMaxLen)
}
