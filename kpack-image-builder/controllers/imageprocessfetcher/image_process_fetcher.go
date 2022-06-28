package imageprocessfetcher

import (
	"encoding/json"
	"fmt"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/utils/net"
)

type ImageProcessFetcher struct {
	Log logr.Logger
}

func (f *ImageProcessFetcher) Fetch(imageRef string, credsOption remote.Option, transport remote.Option) ([]korifiv1alpha1.ProcessType, []int32, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		f.Log.Info(fmt.Sprintf("Error fetching image config: %s\n", err))
		return nil, nil, err
	}

	img, err := remote.Image(ref, credsOption, transport)
	if err != nil {
		f.Log.Info(fmt.Sprintf("Error fetching image config: %s\n", err))
		return nil, nil, err
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		f.Log.Info(fmt.Sprintf("Error fetching image config: %s\n", err))
		return nil, nil, err
	}

	// Unmarshall Build Metadata information from Image Config
	var buildMetadata platform.BuildMetadata
	err = json.Unmarshal([]byte(cfgFile.Config.Labels[platform.BuildMetadataLabel]), &buildMetadata)
	if err != nil {
		f.Log.Info(fmt.Sprintf("Error unmarshalling image build metadata: %s\n", err))
		return nil, nil, err
	}

	// Loop over all the Processes and extract the complete command string
	processTypes := []korifiv1alpha1.ProcessType{}
	for _, process := range buildMetadata.Processes {
		processTypes = append(processTypes, korifiv1alpha1.ProcessType{
			Type:    process.Type,
			Command: extractFullCommand(process),
		})
	}

	exposedPorts, err := extractExposedPorts(&cfgFile.Config)
	if err != nil {
		f.Log.Info(fmt.Sprintf("Cannot parse exposed ports from image config: %v \n", err))
		return nil, nil, err
	}
	return processTypes, exposedPorts, nil
}

// Reconstruct command with arguments into a single command string
func extractFullCommand(process launch.Process) string {
	commandWithArgs := append([]string{process.Command}, process.Args...)
	return strings.Join(commandWithArgs, " ")
}

func extractExposedPorts(imageConfig *v1.Config) ([]int32, error) {
	// Drop the protocol since we only use TCP (the default) and only store the port number
	ports := []int32{}
	for port := range imageConfig.ExposedPorts {
		parsed, err := net.ParsePort(port, false)
		if err != nil {
			return []int32{}, err
		}
		ports = append(ports, int32(parsed))
	}
	return ports, nil
}
