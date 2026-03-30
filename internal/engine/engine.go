package engine

import (
	"fmt"
	"os/exec"
)

// Engine represents a detected container engine (podman or docker).
type Engine struct {
	Name string // "podman" or "docker"
	Path string // absolute path to binary
}

// Options controls engine detection.
type Options struct {
	CLIFlag  string                          // highest priority
	ConfigVal string                         // second priority
	LookPath func(string) (string, error)    // injectable for testing; defaults to exec.LookPath
}

// Detect finds the container engine to use.
// Priority: CLIFlag > ConfigVal > PATH detection (podman first).
func Detect(opts Options) (*Engine, error) {
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	// Priority 1: CLI flag
	if opts.CLIFlag != "" {
		return resolve(opts.CLIFlag, lookPath)
	}

	// Priority 2: Config value
	if opts.ConfigVal != "" {
		return resolve(opts.ConfigVal, lookPath)
	}

	// Priority 3: auto-detect, podman first
	if path, err := lookPath("podman"); err == nil {
		return &Engine{Name: "podman", Path: path}, nil
	}
	if path, err := lookPath("docker"); err == nil {
		return &Engine{Name: "docker", Path: path}, nil
	}

	return nil, fmt.Errorf("neither podman nor docker found in PATH")
}

func resolve(name string, lookPath func(string) (string, error)) (*Engine, error) {
	switch name {
	case "podman", "docker":
		path, err := lookPath(name)
		if err != nil {
			return nil, fmt.Errorf("engine %q not found in PATH: %w", name, err)
		}
		return &Engine{Name: name, Path: path}, nil
	default:
		return nil, fmt.Errorf("unsupported engine %q: must be 'podman' or 'docker'", name)
	}
}

// ComposeCmd returns the command args for running compose.
// Docker uses "docker compose" (plugin preferred), podman uses "podman compose".
func (e *Engine) ComposeCmd(args ...string) []string {
	cmd := []string{e.Path, "compose"}
	return append(cmd, args...)
}

// ProjectCmd returns compose command args with project name, env file, and compose files.
func (e *Engine) ProjectCmd(project string, envFile string, composeFiles []string, args ...string) []string {
	cmd := []string{e.Path, "compose"}
	if project != "" {
		cmd = append(cmd, "-p", project)
	}
	if envFile != "" {
		cmd = append(cmd, "--env-file", envFile)
	}
	for _, f := range composeFiles {
		cmd = append(cmd, "-f", f)
	}
	return append(cmd, args...)
}

// RunCmd returns command args for running a container.
func (e *Engine) RunCmd(args ...string) []string {
	cmd := []string{e.Path, "run"}
	return append(cmd, args...)
}

// BuildCmd returns command args for building an image.
func (e *Engine) BuildCmd(args ...string) []string {
	cmd := []string{e.Path, "build"}
	return append(cmd, args...)
}

// ImageExistsCmd returns command args to check if an image exists.
func (e *Engine) ImageExistsCmd(image string) []string {
	return []string{e.Path, "image", "inspect", image}
}

// NetworkCreateCmd returns command args to create a network.
func (e *Engine) NetworkCreateCmd(name string) []string {
	return []string{e.Path, "network", "create", name}
}

// NetworkExistsCmd returns command args to check if a network exists.
func (e *Engine) NetworkExistsCmd(name string) []string {
	return []string{e.Path, "network", "inspect", name}
}

// InspectHealthCmd returns command args to inspect container health.
func (e *Engine) InspectHealthCmd(container string) []string {
	return []string{e.Path, "inspect", "--format", "{{.State.Health.Status}}", container}
}

// HostGatewayArgs returns additional args needed for host access.
// Docker needs --add-host, podman uses host.containers.internal natively.
func (e *Engine) HostGatewayArgs() []string {
	if e.Name == "docker" {
		return []string{"--add-host", "host-gateway:host-gateway"}
	}
	return nil
}
