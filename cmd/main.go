package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type ServiceConfig struct {
	Image        string                      `yaml:"image"`
	Version      string                      `yaml:"version"`
	Build        *BuildConfig                `yaml:"build,omitempty"`
	Ports        []string                    `yaml:"ports,omitempty"`
	Environment  map[string]string           `yaml:"environment,omitempty"`
	Dependencies map[string]DependencyConfig `yaml:"dependencies,omitempty"`
	DependsOn    []string                    `yaml:"depends_on,omitempty"`
	Command      string                      `yaml:"command,omitempty"`
}

type BuildConfig struct {
	Context          string            `yaml:"context"`
	DockerfileInline string            `yaml:"dockerfile_inline"`
	Args             map[string]string `yaml:"args,omitempty"`
}

type DependencyConfig struct {
	Image       string            `yaml:"image,omitempty"`
	Version     string            `yaml:"version"`
	Internal    bool              `yaml:"internal"`
	Expose      int               `yaml:"expose,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

type ComposeService struct {
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports,omitempty"`
	Expose      []string          `yaml:"expose,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Networks    []string          `yaml:"networks,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`
	Command     string            `yaml:"command,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
}

type ComposeConfig struct {
	Version  string                     `yaml:"version,omitempty"`
	Services map[string]ComposeService  `yaml:"services"`
	Networks map[string]map[string]bool `yaml:"networks,omitempty"`
}

func main() {
	services := loadServiceConfig()
	portsConf := loadPortConfig()
	plugins := loadPlugins()

	filtered := make(map[string]ServiceConfig)
	for name, svc := range services {
		if !plugins[name] {
			continue
		}
		if p, ok := portsConf[name]; ok {
			svc.Ports = p
		}
		filtered[name] = svc
	}

	secrets := generateSecrets(filtered)
	writeEnvFile(secrets)
	buildImages(filtered)
	compose := buildCompose(filtered, secrets)
	writeComposeFile(compose)
	fmt.Println("Configuration generated.")
}

func buildImages(services map[string]ServiceConfig) {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	built := make(map[string]bool)
	for _, name := range names {
		svc := services[name]
		if svc.Build == nil {
			continue
		}
		image := fmt.Sprintf("%s:%s", svc.Image, svc.Version)
		if built[image] {
			continue
		}

		args := dockerBuildArgs(image, *svc.Build)
		cmd := exec.Command("docker", args...)
		cmd.Stdin = strings.NewReader(svc.Build.DockerfileInline)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("Building %s...\n", image)
		if err := cmd.Run(); err != nil {
			panic(fmt.Errorf("build %s: %w", image, err))
		}
		built[image] = true
	}
}

func dockerBuildArgs(image string, build BuildConfig) []string {
	args := []string{"build"}
	keys := make([]string, 0, len(build.Args))
	for key := range build.Args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--build-arg", key+"="+build.Args[key])
	}
	return append(args, "-t", image, "-f", "-", build.Context)
}

func loadServiceConfig() map[string]ServiceConfig {
	data, err := os.ReadFile("config/services.yaml")
	if err != nil {
		panic(err)
	}
	var root struct {
		Services map[string]ServiceConfig `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		panic(err)
	}
	return root.Services
}

func loadPortConfig() map[string][]string {
	data, err := os.ReadFile("config/ports.yaml")
	if err != nil {
		return nil
	}
	var root struct {
		Services map[string][]string `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		panic(err)
	}
	return root.Services
}

func loadPlugins() map[string]bool {
	cfg, err := ini.Load("plugins.conf")
	if err != nil {
		panic(err)
	}
	enabled := make(map[string]bool)
	for _, key := range cfg.Section("services").KeyStrings() {
		if v := cfg.Section("services").Key(key).String(); v == "enabled" {
			enabled[key] = true
		}
	}
	return enabled
}

func generateSecrets(services map[string]ServiceConfig) map[string]string {
	existing := loadDotEnv()
	secrets := make(map[string]string)
	for _, svc := range services {
		for k, v := range svc.Environment {
			if strings.HasPrefix(v, "{{GENERATE_") {
				if value, ok := existing[k]; ok {
					secrets[k] = value
					continue
				}
				secrets[k] = generateRandomString(parseLength(v))
			}
		}
		for _, dep := range svc.Dependencies {
			for k, v := range dep.Environment {
				if strings.HasPrefix(v, "{{GENERATE_") {
					if value, ok := existing[k]; ok {
						secrets[k] = value
						continue
					}
					secrets[k] = generateRandomString(parseLength(v))
				}
			}
		}
	}
	return secrets
}

func loadDotEnv() map[string]string {
	env := make(map[string]string)
	data, err := os.ReadFile(".env")
	if err != nil {
		return env
	}
	for _, line := range strings.Split(string(data), "\n") {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func writeEnvFile(env map[string]string) {
	var b strings.Builder
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("%s=%s\n", k, env[k]))
	}
	if err := os.WriteFile(".env", []byte(b.String()), 0644); err != nil {
		panic(err)
	}
}

func buildCompose(services map[string]ServiceConfig, secrets map[string]string) ComposeConfig {
	cfg := ComposeConfig{
		Services: make(map[string]ComposeService),
		Networks: map[string]map[string]bool{"default": {}},
	}
	for name, svc := range services {
		svcNet := fmt.Sprintf("%s_internal", name)
		if len(svc.Dependencies) > 0 {
			cfg.Networks[svcNet] = map[string]bool{"internal": true}
		}

		cs := ComposeService{
			Image:       fmt.Sprintf("%s:%s", svc.Image, svc.Version),
			Ports:       svc.Ports,
			Environment: make(map[string]string),
			Networks:    []string{"default"},
			Restart:     "always",
			Command:     svc.Command,
		}
		cs.DependsOn = append(cs.DependsOn, svc.DependsOn...)
		for k, v := range svc.Environment {
			cs.Environment[k] = resolveEnvValue(k, v, secrets)
		}

		if len(svc.Dependencies) > 0 {
			cs.Networks = append(cs.Networks, svcNet)
			depKeys := make([]string, 0, len(svc.Dependencies))
			for depKey := range svc.Dependencies {
				depKeys = append(depKeys, depKey)
			}
			sort.Strings(depKeys)
			for _, depKey := range depKeys {
				depCfg := svc.Dependencies[depKey]
				depName := fmt.Sprintf("%s_%s", name, depKey)
				cs.DependsOn = append(cs.DependsOn, depName)
				cs.Environment[strings.ToUpper(depKey)+"_HOST"] = depName
				cs.Environment[strings.ToUpper(depKey)+"_PORT"] = strconv.Itoa(depCfg.Expose)
				depEnv := make(map[string]string)
				for k, v := range depCfg.Environment {
					depEnv[k] = resolveEnvValue(k, v, secrets)
				}

				cfg.Services[depName] = ComposeService{
					Image:       fmt.Sprintf("%s:%s", depCfg.ImageOrKey(), depCfg.Version),
					Expose:      []string{strconv.Itoa(depCfg.Expose)},
					Environment: depEnv,
					Networks:    []string{svcNet},
					Restart:     "always",
				}
			}
		}

		cfg.Services[name] = cs
	}
	return cfg
}

func resolveEnvValue(key, value string, secrets map[string]string) string {
	if strings.HasPrefix(value, "{{GENERATE_") {
		if secret, ok := secrets[key]; ok {
			return secret
		}
	}
	return value
}

func (d DependencyConfig) ImageOrKey() string {
	if d.Image != "" {
		return d.Image
	}
	return ""
}

func writeComposeFile(compose ComposeConfig) {
	out, err := yaml.Marshal(compose)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("docker-compose.yml", out, 0644); err != nil {
		panic(err)
	}
}

func parseLength(s string) int {
	val := strings.TrimSuffix(strings.TrimPrefix(s, "{{GENERATE_"), "}}")
	n, _ := strconv.Atoi(val)
	return n
}

func generateRandomString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)[:n]
}
