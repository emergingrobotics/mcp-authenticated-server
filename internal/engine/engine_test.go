package engine

import (
	"fmt"
	"testing"
)

func mockLookPath(available map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := available[name]; ok {
			return path, nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}
}

func TestDetect_PodmanPreferred(t *testing.T) {
	e, err := Detect(Options{
		LookPath: mockLookPath(map[string]string{
			"podman": "/usr/bin/podman",
			"docker": "/usr/bin/docker",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "podman" {
		t.Errorf("expected podman, got %s", e.Name)
	}
}

func TestDetect_DockerFallback(t *testing.T) {
	e, err := Detect(Options{
		LookPath: mockLookPath(map[string]string{
			"docker": "/usr/bin/docker",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "docker" {
		t.Errorf("expected docker, got %s", e.Name)
	}
}

func TestDetect_NeitherFound(t *testing.T) {
	_, err := Detect(Options{
		LookPath: mockLookPath(map[string]string{}),
	})
	if err == nil {
		t.Fatal("expected error when neither engine found")
	}
}

func TestDetect_CLIFlagOverride(t *testing.T) {
	e, err := Detect(Options{
		CLIFlag: "docker",
		LookPath: mockLookPath(map[string]string{
			"podman": "/usr/bin/podman",
			"docker": "/usr/bin/docker",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "docker" {
		t.Errorf("expected docker (CLI flag), got %s", e.Name)
	}
}

func TestDetect_ConfigOverride(t *testing.T) {
	e, err := Detect(Options{
		ConfigVal: "docker",
		LookPath: mockLookPath(map[string]string{
			"podman": "/usr/bin/podman",
			"docker": "/usr/bin/docker",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "docker" {
		t.Errorf("expected docker (config), got %s", e.Name)
	}
}

func TestDetect_CLIFlagPriorityOverConfig(t *testing.T) {
	e, err := Detect(Options{
		CLIFlag:   "podman",
		ConfigVal: "docker",
		LookPath: mockLookPath(map[string]string{
			"podman": "/usr/bin/podman",
			"docker": "/usr/bin/docker",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "podman" {
		t.Errorf("expected podman (CLI flag priority), got %s", e.Name)
	}
}

func TestDetect_InvalidEngine(t *testing.T) {
	_, err := Detect(Options{
		CLIFlag:  "containerd",
		LookPath: mockLookPath(map[string]string{}),
	})
	if err == nil {
		t.Fatal("expected error for invalid engine name")
	}
}

func TestComposeCmd(t *testing.T) {
	e := &Engine{Name: "podman", Path: "/usr/bin/podman"}
	got := e.ComposeCmd("up", "-d")
	want := []string{"/usr/bin/podman", "compose", "up", "-d"}
	assertSliceEqual(t, got, want)
}

func TestProjectCmd(t *testing.T) {
	e := &Engine{Name: "docker", Path: "/usr/bin/docker"}
	got := e.ProjectCmd("myproject", ".env", []string{"compose.yml"}, "up", "-d")
	want := []string{"/usr/bin/docker", "compose", "-p", "myproject", "--env-file", ".env", "-f", "compose.yml", "up", "-d"}
	assertSliceEqual(t, got, want)
}

func TestRunCmd(t *testing.T) {
	e := &Engine{Name: "podman", Path: "/usr/bin/podman"}
	got := e.RunCmd("--rm", "alpine", "echo", "hello")
	want := []string{"/usr/bin/podman", "run", "--rm", "alpine", "echo", "hello"}
	assertSliceEqual(t, got, want)
}

func TestBuildCmd(t *testing.T) {
	e := &Engine{Name: "docker", Path: "/usr/bin/docker"}
	got := e.BuildCmd("-t", "myimage", ".")
	want := []string{"/usr/bin/docker", "build", "-t", "myimage", "."}
	assertSliceEqual(t, got, want)
}

func TestHostGatewayArgs_Docker(t *testing.T) {
	e := &Engine{Name: "docker", Path: "/usr/bin/docker"}
	got := e.HostGatewayArgs()
	if len(got) != 2 {
		t.Fatalf("expected 2 args for docker, got %d", len(got))
	}
	if got[0] != "--add-host" {
		t.Errorf("expected --add-host, got %s", got[0])
	}
}

func TestHostGatewayArgs_Podman(t *testing.T) {
	e := &Engine{Name: "podman", Path: "/usr/bin/podman"}
	got := e.HostGatewayArgs()
	if got != nil {
		t.Errorf("expected nil for podman, got %v", got)
	}
}

func assertSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
