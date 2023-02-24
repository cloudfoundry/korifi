package imageprocessfetcher

import (
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/utils/net"
)

const (
	buildpackBuildMetadataLabel = "io.buildpacks.build.metadata"
)

type ImageProcessFetcher struct {
	Log logr.Logger
}

type buildMetadata struct {
	Processes []process `json:"processes"`
}

type process struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (f *ImageProcessFetcher) Fetch(imageRef string, credsOption remote.Option) ([]korifiv1alpha1.ProcessType, []int32, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		f.Log.Info("error fetching image config", "reason", err)
		return nil, nil, err
	}

	img, err := remote.Image(ref, credsOption)
	if err != nil {
		f.Log.Info("error fetching image config", "reason", err)
		return nil, nil, err
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		f.Log.Info("error fetching image config", "reason", err)
		return nil, nil, err
	}

	// Unmarshall Build Metadata information from Image Config
	var buildMd buildMetadata
	err = json.Unmarshal([]byte(cfgFile.Config.Labels[buildpackBuildMetadataLabel]), &buildMd)
	if err != nil {
		f.Log.Info("error unmarshalling image build metadata", "reason", err)
		return nil, nil, err
	}

	// Loop over all the Processes and extract the complete command string
	processTypes := []korifiv1alpha1.ProcessType{}
	for _, process := range buildMd.Processes {
		processTypes = append(processTypes, korifiv1alpha1.ProcessType{
			Type:    process.Type,
			Command: extractFullCommand(process),
		})
	}

	exposedPorts, err := extractExposedPorts(&cfgFile.Config)
	if err != nil {
		f.Log.Info("cannot parse exposed ports from image config", "reason", err)
		return nil, nil, err
	}
	return processTypes, exposedPorts, nil
}

// Reconstruct command with arguments into a single command string
func extractFullCommand(process process) string {
	cmdString := process.Command
	for _, a := range process.Args {
		cmdString = fmt.Sprintf(`%s %q`, cmdString, a)
	}
	return cmdString
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
