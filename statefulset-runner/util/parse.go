package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const DockerHubHost = "index.docker.io/v1/"

var dockerRX = regexp.MustCompile(`([a-zA-Z0-9.-]+)(:([0-9]+))?/(\S+/\S+)`)

func ParseAppIndex(podName string) (int, error) {
	sl := strings.Split(podName, "-")

	if len(sl) <= 1 {
		return 0, fmt.Errorf("could not parse app name from %s", podName)
	}

	index, err := strconv.Atoi(sl[len(sl)-1])
	if err != nil {
		return 0, errors.Wrapf(err, "pod %s name does not contain an index", podName)
	}

	return index, nil
}

func ParseImageRegistryHost(imageURL string) string {
	if !dockerRX.MatchString(imageURL) {
		return DockerHubHost
	}

	matches := dockerRX.FindStringSubmatch(imageURL)

	return matches[1]
}
