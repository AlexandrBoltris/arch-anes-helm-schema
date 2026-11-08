package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/arch-anes/helm-schema/internal/helmchart"
)

func TestDeploymentEngines(t *testing.T) {
	for _, engine := range []string{"werf", "nelm"} {
		t.Run(engine, func(t *testing.T) {
			directory := t.TempDir()
			chartPath := directory
			if engine == "werf" {
				chartPath = filepath.Join(directory, ".helm")
			}
			files := map[string]string{
				"Chart.yaml":                         "apiVersion: v2\nname: app\nversion: 1.0.0\ndependencies:\n- name: child\n  version: 1.0.0\n  repository: file://charts/child\n",
				"values.yaml":                        "replicas: 1\nunused: fixed\nglobal:\n  ordinary: fixed\n  env: stub\n  werf:\n    env: stub\nwerf:\n  image:\n    backend: stub\ndockerconfigjson: stub\n",
				"templates/config.yaml":              "replicas: {{ .Values.replicas }}\nimage: {{ .Values.werf.image.backend }}\nenv: {{ .Values.global.werf.env }}\nsecret: {{ werf_secret_file (print .Values.secretFile) }}\n",
				"charts/child/Chart.yaml":            "apiVersion: v2\nname: child\nversion: 1.0.0\n",
				"charts/child/values.yaml":           "enabled: true\n",
				"charts/child/templates/config.yaml": "enabled: {{ .Values.enabled }}\n",
			}
			for name, content := range files {
				target := filepath.Join(chartPath, name)
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"--engine", engine, directory}
			if err := run(args, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{chartPath, filepath.Join(chartPath, "charts/child")} {
				content, err := os.ReadFile(filepath.Join(path, "values.schema.json"))
				if err != nil {
					t.Fatal(err)
				}
				values := map[string]any{
					"werf":             map[string]any{"image": map[string]any{"backend": "registry/app:tag", "new": "registry/new:tag"}, "env": "prod"},
					"global":           map[string]any{"env": "prod", "werf": map[string]any{"env": "prod", "images": map[string]any{"new": map[string]any{"tag": "v1"}}}},
					"dockerconfigjson": "runtime registry credentials",
				}
				for _, env := range []string{"", "prod"} {
					values["global"].(map[string]any)["env"] = env
					if err := helmchart.Validate(values, content); err != nil {
						t.Fatal(err)
					}
				}
				for _, invalid := range []map[string]any{
					{"typo": true}, {"unused": "changed"}, {"global": map[string]any{"typo": true}},
				} {
					if err := helmchart.Validate(invalid, content); err == nil {
						t.Fatalf("accepted unsupported values: %#v", invalid)
					}
				}
				if path == chartPath {
					if err := helmchart.Validate(map[string]any{"global": map[string]any{"ordinary": "changed"}}, content); err == nil {
						t.Fatal("opened unrelated global default")
					}
				}
				var schema map[string]any
				if err := json.Unmarshal(content, &schema); err != nil {
					t.Fatal(err)
				}
				if schema["additionalProperties"] != false {
					t.Fatal("opened values root")
				}
			}
			if err := run([]string{"--engine", engine, "--check", directory}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnknownEngine(t *testing.T) {
	if err := run([]string{"--engine", "other"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted unknown engine")
	}
}
func TestDeploymentChartPath(t *testing.T) {
	directory := t.TempDir()
	hidden := filepath.Join(directory, ".helm")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ engine, want string }{
		{"helm", directory}, {"nelm", directory}, {"werf", hidden},
	} {
		got, err := deploymentChartPath(directory, test.engine)
		if err != nil || got != test.want {
			t.Fatalf("%s: got %q, %v; want %q", test.engine, got, err, test.want)
		}
	}
	got, err := deploymentChartPath(hidden, "werf")
	if err != nil || got != hidden {
		t.Fatalf("explicit chart: got %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "Chart.yaml"), []byte("apiVersion: v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = deploymentChartPath(directory, "werf")
	if err != nil || got != directory {
		t.Fatalf("root chart precedence: got %q, %v", got, err)
	}
}

func TestHelmDoesNotPermitServiceValues(t *testing.T) {
	directory := copyBasicChart(t)
	if err := run([]string{directory}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "values.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range []map[string]any{
		{"werf": map[string]any{"env": "prod"}},
		{"global": map[string]any{"env": "prod"}},
	} {
		if err := helmchart.Validate(values, content); err == nil {
			t.Fatalf("Helm allowed service values: %#v", values)
		}
	}
}
