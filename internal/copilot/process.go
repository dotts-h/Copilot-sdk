package copilot

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// SpawnConfig configures launching the Node sidecar process.
type SpawnConfig struct {
	Command     string
	Args        []string
	WorkingDir  string
	GitHubToken string
	// ExtraEnv is appended to the inherited environment.
	ExtraEnv []string
	// Stderr, if non-nil, receives the sidecar's stderr (else os.Stderr).
	Stderr io.Writer
}

// Spawn launches the sidecar and returns a StdioClient wired to its stdio. The
// returned client's Close terminates the process.
func Spawn(cfg SpawnConfig) (*StdioClient, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("copilot: empty sidecar command")
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Dir = cfg.WorkingDir
	cmd.Env = append(os.Environ(), cfg.ExtraEnv...)
	if cfg.GitHubToken != "" {
		cmd.Env = append(cmd.Env, "GITHUB_TOKEN="+cfg.GitHubToken)
	}
	if cfg.Stderr != nil {
		cmd.Stderr = cfg.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sidecar %q: %w", cfg.Command, err)
	}

	stop := func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}
	return NewStdioClient(stdout, stdin, stop), nil
}
