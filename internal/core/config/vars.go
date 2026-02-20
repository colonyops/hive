package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// loadVarsFiles reads YAML vars files and merges them in declaration order.
// Later files override earlier files for the same keys.
func loadVarsFiles(configDir string, files []string) (map[string]any, error) {
	merged := make(map[string]any)

	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read vars file %q: %w", file, err)
		}

		var vars map[string]any
		if err := yaml.Unmarshal(data, &vars); err != nil {
			return nil, fmt.Errorf("parse vars file %q: %w", file, err)
		}

		mergeMaps(merged, vars)
	}

	return merged, nil
}

// mergeMaps recursively merges src into dst.
// Nested maps are merged, while scalar and non-map values are replaced.
func mergeMaps(dst, src map[string]any) {
	if src == nil {
		return
	}

	for key, srcVal := range src {
		srcMap, srcIsMap := srcVal.(map[string]any)
		if !srcIsMap {
			dst[key] = srcVal
			continue
		}

		dstMap, dstIsMap := dst[key].(map[string]any)
		if !dstIsMap {
			dst[key] = srcMap
			continue
		}

		mergeMaps(dstMap, srcMap)
	}
}
