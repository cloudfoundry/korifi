package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

func LoadConfigInto(conf any, fromDir string) error {
	items, err := os.ReadDir(fromDir)
	if err != nil {
		return fmt.Errorf("error reading config dir %q: %w", fromDir, err)
	}

	for _, item := range items {
		fileName := item.Name()
		if item.IsDir() || strings.HasPrefix(fileName, ".") {
			continue
		}

		configFile, err := os.Open(filepath.Join(fromDir, fileName))
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer configFile.Close()

		decoder := yaml.NewDecoder(configFile)
		if err = decoder.Decode(conf); err != nil {
			return fmt.Errorf("failed decoding %q: %w", item.Name(), err)
		}
	}

	return nil
}
