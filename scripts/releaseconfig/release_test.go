package releaseconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func load(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAssetStagingAndShellSyntax(t *testing.T) {
	bash := "C:/Program Files/Git/bin/bash.exe"
	if _, err := os.Stat(bash); err != nil {
		bash = "bash"
	}
	jobs := load(t, ".github/workflows/reusable-fork-release.yml")["jobs"].(map[string]interface{})
	for _, job := range jobs {
		steps, ok := job.(map[string]interface{})["steps"].([]interface{})
		if !ok {
			continue
		}
		for _, step := range steps {
			command, ok := step.(map[string]interface{})["run"].(string)
			if !ok {
				continue
			}
			check := exec.Command(bash, "-n")
			check.Stdin = strings.NewReader(command)
			if output, err := check.CombinedOutput(); err != nil {
				t.Fatalf("shell syntax: %v: %s", err, output)
			}
		}
	}
	steps := jobs["release"].(map[string]interface{})["steps"].([]interface{})
	var staging string
	for _, step := range steps {
		s := step.(map[string]interface{})
		if s["name"] == "Verify assets and version" {
			staging = s["run"].(string)
		}
	}
	if staging == "" {
		t.Fatal("missing staging step")
	}
	for _, duplicate := range []bool{false, true} {
		root := t.TempDir()
		for _, osname := range []string{"linux", "darwin", "windows"} {
			for _, arch := range []string{"amd64", "arm64"} {
				name := "pinchtab-" + osname + "-" + arch
				if osname == "windows" {
					name += ".exe"
				}
				dir := filepath.Join(root, "dist", osname+"_"+arch)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				data := []byte("fixture")
				if osname == "linux" && arch == "amd64" {
					data = []byte(strings.Join([]string{"#!/bin/sh", "echo pinchtab 0.15.3", ""}, string(rune(10))))
				}
				if err := os.WriteFile(filepath.Join(dir, name), data, 0755); err != nil {
					t.Fatal(err)
				}
				if duplicate && osname == "linux" && arch == "amd64" {
					os.MkdirAll(filepath.Join(root, "dist", "duplicate"), 0755)
					os.WriteFile(filepath.Join(root, "dist", "duplicate", name), data, 0755)
				}
			}
		}
		command := exec.Command(bash, "-c", staging)
		command.Dir = root
		command.Env = append(os.Environ(), "RELEASE_VERSION=0.15.3")
		output, err := command.CombinedOutput()
		if duplicate {
			if err == nil {
				t.Fatal("duplicate binary accepted")
			}
			continue
		}
		if err != nil {
			t.Fatalf("asset staging: %v: %s", err, output)
		}
		lines, err := os.ReadFile(filepath.Join(root, "release-assets", "checksums.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.Fields(string(lines))) != 12 {
			t.Fatal("expected six SHA256 entries")
		}
	}
}

func TestForkReleaseConfiguration(t *testing.T) {
	for _, path := range []string{".github/workflows/release.yml", ".github/workflows/reusable-fork-release.yml", ".github/workflows/reusable-release-publish.yml", ".goreleaser.fork.yml"} {
		load(t, path)
	}
	cfg := load(t, ".goreleaser.fork.yml")
	for _, field := range []string{"brews", "dockers", "nfpms", "scoops", "publishers"} {
		if _, ok := cfg[field]; ok {
			t.Fatalf("external publication field: %s", field)
		}
	}
	builds := cfg["builds"].([]interface{})
	if len(builds) != 1 {
		t.Fatal("expected one build matrix")
	}
	build := builds[0].(map[string]interface{})
	if build["binary"] != "pinchtab-{{ .Os }}-{{ .Arch }}" {
		t.Fatal("binary names must match asset staging")
	}
	if cfg["snapshot"].(map[string]interface{})["version_template"] != "{{ .Env.RELEASE_VERSION }}" {
		t.Fatal("snapshot must use persisted release version")
	}
	if len(build["goos"].([]interface{})) != 3 || len(build["goarch"].([]interface{})) != 2 {
		t.Fatal("expected six targets")
	}
	if !strings.Contains(build["ldflags"].([]interface{})[0].(string), "main.version={{.Version}}") {
		t.Fatal("missing version injection")
	}
	entry := load(t, ".github/workflows/release.yml")["jobs"].(map[string]interface{})
	if entry["fork-release"].(map[string]interface{})["if"] != "github.repository == 'omrehbein/pinchtab'" {
		t.Fatal("missing fork entry guard")
	}
	if entry["validate"].(map[string]interface{})["if"] != "github.repository == 'pinchtab/pinchtab'" {
		t.Fatal("upstream entry not guarded")
	}
	upstream := load(t, ".github/workflows/reusable-release-publish.yml")["jobs"].(map[string]interface{})
	if upstream["release"].(map[string]interface{})["if"] != "github.repository == 'pinchtab/pinchtab'" {
		t.Fatal("upstream publisher not guarded")
	}
	jobs := load(t, ".github/workflows/reusable-fork-release.yml")["jobs"].(map[string]interface{})
	release := jobs["release"].(map[string]interface{})
	needs := release["needs"].([]interface{})
	if len(needs) != 6 {
		t.Fatal("missing release prerequisites")
	}
	for _, dependency := range needs {
		if _, ok := jobs[dependency.(string)]; !ok {
			t.Fatal("unknown prerequisite")
		}
	}
}

func TestReusableWorkflowContracts(t *testing.T) {
	jobs := load(t, ".github/workflows/reusable-fork-release.yml")["jobs"].(map[string]interface{})
	for name, value := range jobs {
		job := value.(map[string]interface{})
		path, ok := job["uses"].(string)
		if !ok {
			continue
		}
		doc := load(t, strings.TrimPrefix(path, "./"))
		call := doc["on"].(map[string]interface{})["workflow_call"]
		inputs := map[string]interface{}{}
		if c, ok := call.(map[string]interface{}); ok {
			if i, ok := c["inputs"].(map[string]interface{}); ok {
				inputs = i
			}
		}
		provided, _ := job["with"].(map[string]interface{})
		for key := range provided {
			if _, ok := inputs[key]; !ok {
				t.Fatalf("%s passes unsupported input %s", name, key)
			}
		}
		for key, spec := range inputs {
			if spec.(map[string]interface{})["required"] == true {
				if _, ok := provided[key]; !ok {
					t.Fatalf("%s missing required input %s", name, key)
				}
			}
		}
	}
}
