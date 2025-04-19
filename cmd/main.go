package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

// ServiceConfig represents a service entry in services.yaml
type ServiceConfig struct {
	Image        string                      `yaml:"image"`
	Version      string                      `yaml:"version"`
	Ports        []string                    `yaml:"ports,omitempty"`
	Environment  map[string]string           `yaml:"environment,omitempty"`
	Dependencies map[string]DependencyConfig `yaml:"dependencies,omitempty"`
	Command      string                      `yaml:"command,omitempty"`
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
	// Load all possible services and port overrides
	services := loadServiceConfig()
	portsConf := loadPortConfig()
	plugins := loadPlugins()

	// Filter by enabled plugins and apply port overrides
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

	// Generate .env and compose
	env := generateEnvVars(filtered)
	writeEnvFile(env)
	compose := buildCompose(filtered, env)
	writeComposeFile(compose)
	fmt.Println("✅ Configuration generated and containers started!")
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

func generateEnvVars(services map[string]ServiceConfig) map[string]string {
	existing := loadDotEnv()
	env := make(map[string]string)
	for _, svc := range services {
		// top-level env
		for k, v := range svc.Environment {
			if strings.HasPrefix(v, "{{GENERATE_") {
				if _, ok := existing[k]; !ok {
					existing[k] = generateRandomString(parseLength(v))
				}
				env[k] = existing[k]
			} else {
				env[k] = v
			}
		}
		// dependencies env
		for _, dep := range svc.Dependencies {
			for k, v := range dep.Environment {
				if strings.HasPrefix(v, "{{GENERATE_") {
					if _, ok := existing[k]; !ok {
						existing[k] = generateRandomString(parseLength(v))
					}
					env[k] = existing[k]
				} else {
					env[k] = v
				}
			}
		}
	}
	return env
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
	for k, v := range env {
		b.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}
	os.WriteFile(".env", []byte(b.String()), 0644)
}

func buildCompose(services map[string]ServiceConfig, env map[string]string) ComposeConfig {
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

		if len(svc.Dependencies) > 0 {
			cs.Networks = append(cs.Networks, svcNet)
			for depKey, depCfg := range svc.Dependencies {
				depName := fmt.Sprintf("%s_%s", name, depKey)
				cs.DependsOn = append(cs.DependsOn, depName)
				cs.Environment[strings.ToUpper(depKey)+"_HOST"] = depName
				cs.Environment[strings.ToUpper(depKey)+"_PORT"] = strconv.Itoa(depCfg.Expose)

				// internal dependency service
				depEnv := make(map[string]string)
				for k := range depCfg.Environment {
					depEnv[k] = env[k]
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

		// inline top-level ENV
		for k := range svc.Environment {
			cs.Environment[k] = env[k]
		}
		cfg.Services[name] = cs
	}
	return cfg
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
	os.WriteFile("docker-compose.yml", out, 0644)
}

func parseLength(s string) int {
	val := strings.TrimSuffix(strings.TrimPrefix(s, "{{GENERATE_"), "}}")
	n, _ := strconv.Atoi(val)
	return n
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:n]
}
