package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arch-anes/helm-schema/internal/analyze"
	"github.com/arch-anes/helm-schema/internal/helmchart"
)

// deploymentChartPath accepts a werf project directory as well as an explicit
// chart directory. An existing Chart.yaml always takes precedence over .helm.
func deploymentChartPath(directory, engine string) (string, error) {
	if engine != "werf" {
		return directory, nil
	}
	if _, err := os.Stat(filepath.Join(directory, "Chart.yaml")); err == nil {
		return directory, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect chart directory: %w", err)
	}
	candidate := filepath.Join(directory, ".helm")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect werf chart directory: %w", err)
	}
	return directory, nil
}

// allowServiceValues reserves only engine-owned namespaces in every dependency
// and alias scope. Run after value flows so imported user values stay strict.
func allowServiceValues(usage *analyze.Usage, scopes []helmchart.ChartScope) {
	for _, scope := range scopes {
		for _, prefix := range scope.ValuesPrefixes {
			for _, path := range [][]string{{"werf"}, {"global", "werf"}, {"global", "env"}, {"dockerconfigjson"}} {
				node := usage
				for _, name := range append(append([]string{}, prefix...), path[:len(path)-1]...) {
					if node.Properties == nil {
						node.Properties = make(map[string]*analyze.Usage)
					}
					if node.Properties[name] == nil {
						node.Properties[name] = analyze.NewUsage()
					}
					node = node.Properties[name]
				}
				if node.Properties == nil {
					node.Properties = make(map[string]*analyze.Usage)
				}
				name := path[len(path)-1]
				service := &analyze.Usage{Open: true, Serialized: true}
				if previous := node.Properties[name]; previous != nil {
					service.Sources = previous.Sources
				}
				node.Properties[name] = service
			}
		}
	}
}
