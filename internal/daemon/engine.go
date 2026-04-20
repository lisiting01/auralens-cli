package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Engine runs an AI tool with a prompt and returns the result text.
type Engine interface {
	Run(ctx context.Context, prompt string, workDir string) (string, error)
	Name() string
}

// StreamingEngine extends Engine with a method that pipes subprocess output
// directly to the caller instead of parsing it. Used by `auralens agent run`.
type StreamingEngine interface {
	Engine
	RunStreaming(ctx context.Context, prompt string, workDir string, out io.Writer) (*exec.Cmd, error)
}

// NewEngine creates the appropriate engine based on the configured name.
// extraEnv is an optional map of KEY→VALUE pairs injected into the child
// process environment on top of the current process's environment; config
// values take highest priority and can override OS-level env vars.
func NewEngine(name, path, model string, extraArgs []string, extraEnv map[string]string) Engine {
	switch strings.ToLower(name) {
	case "claude", "claude-code":
		return &ClaudeEngine{path: path, model: model, extraArgs: extraArgs, extraEnv: extraEnv}
	case "codex":
		return &CodexEngine{path: path, extraArgs: extraArgs, extraEnv: extraEnv}
	default:
		return &GenericEngine{path: path, extraArgs: extraArgs, extraEnv: extraEnv}
	}
}

// buildEnv constructs a deduplicated environment slice for a child process.
// Priority (highest last, i.e. last writer wins):
//  1. Current process environment (os.Environ)
//  2. Engine-specific vars (ANTHROPIC_MODEL, CLAUDE_CODE_GIT_BASH_PATH)
//  3. extraEnv from config — overrides everything above
func buildEnv(model string, gitBashPath string, extraEnv map[string]string) []string {
	envMap := make(map[string]string, len(os.Environ())+len(extraEnv)+2)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			envMap[k] = v
		}
	}
	if model != "" {
		envMap["ANTHROPIC_MODEL"] = model
	}
	// Only set git-bash path if not already present in the environment.
	if gitBashPath != "" {
		if _, alreadySet := envMap["CLAUDE_CODE_GIT_BASH_PATH"]; !alreadySet {
			envMap["CLAUDE_CODE_GIT_BASH_PATH"] = gitBashPath
		}
	}
	// Config env takes highest priority — overwrites everything above.
	for k, v := range extraEnv {
		envMap[k] = v
	}
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}
	return result
}

// ── Claude Code Engine ───────────────────────────────────────────────────────

type ClaudeEngine struct {
	path      string
	model     string
	extraArgs []string
	extraEnv  map[string]string
}

func (e *ClaudeEngine) Name() string { return "claude" }

func (e *ClaudeEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	args := []string{"--print", "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	args = append(args, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)
	cmd.Env = buildEnv(e.model, resolveGitBashPath(), e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	result, parseErr := parseClaudeOutput(stdout)
	waitErr := cmd.Wait()

	if parseErr != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return "", fmt.Errorf("%w (stderr: %s)", parseErr, stderr)
		}
		return "", parseErr
	}
	if waitErr != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return result, fmt.Errorf("claude exited with error: %w (stderr: %s)", waitErr, stderr)
		}
	}
	return result, nil
}

// RunStreaming spawns Claude Code and pipes its output directly to out.
func (e *ClaudeEngine) RunStreaming(ctx context.Context, prompt string, workDir string, out io.Writer) (*exec.Cmd, error) {
	args := []string{"--print", "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	args = append(args, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)
	cmd.Env = buildEnv(e.model, resolveGitBashPath(), e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	return cmd, nil
}

type claudeJSONLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	Message *struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func parseClaudeOutput(r io.Reader) (string, error) {
	var lastAssistantText string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg claudeJSONLine
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "result":
			if msg.Subtype == "success" && msg.Result != "" {
				return msg.Result, nil
			}
		case "assistant":
			if msg.Message != nil {
				for _, c := range msg.Message.Content {
					if c.Type == "text" && c.Text != "" {
						lastAssistantText = c.Text
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return lastAssistantText, fmt.Errorf("read stdout: %w", err)
	}
	if lastAssistantText != "" {
		return lastAssistantText, nil
	}
	return "", fmt.Errorf("no result found in claude output")
}

// ── Codex Engine ─────────────────────────────────────────────────────────────

type CodexEngine struct {
	path      string
	extraArgs []string
	extraEnv  map[string]string
}

func (e *CodexEngine) Name() string { return "codex" }

func (e *CodexEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	args := append([]string{"--quiet"}, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)
	cmd.Env = buildEnv("", "", e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start codex: %w", err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	result := parseCodexOutput(stdout)
	_ = cmd.Wait()
	if result == "" {
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			return "", fmt.Errorf("no result from codex (stderr: %s)", s)
		}
		return "", fmt.Errorf("no result from codex")
	}
	return result, nil
}

// RunStreaming spawns Codex CLI and pipes its output directly to out.
func (e *CodexEngine) RunStreaming(ctx context.Context, prompt string, workDir string, out io.Writer) (*exec.Cmd, error) {
	args := append([]string{"--quiet"}, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)
	cmd.Env = buildEnv("", "", e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	return cmd, nil
}

type codexJSONLine struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Output  string `json:"output"`
}

func parseCodexOutput(r io.Reader) string {
	var last string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg codexJSONLine
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Output != "" {
			last = msg.Output
		} else if msg.Content != "" {
			last = msg.Content
		}
	}
	return last
}

// ── Generic Engine ───────────────────────────────────────────────────────────

type GenericEngine struct {
	path      string
	extraArgs []string
	extraEnv  map[string]string
}

func (e *GenericEngine) Name() string { return e.path }

func (e *GenericEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	cmd := newCmd(ctx, e.path, e.extraArgs, workDir)
	cmd.Env = buildEnv("", "", e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start engine %s: %w", e.path, err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	out, err := io.ReadAll(stdout)
	_ = cmd.Wait()
	if err != nil {
		return "", fmt.Errorf("read output: %w", err)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			return "", fmt.Errorf("engine returned empty output (stderr: %s)", s)
		}
		return "", fmt.Errorf("engine returned empty output")
	}
	return result, nil
}

// RunStreaming for GenericEngine pipes output directly to out.
func (e *GenericEngine) RunStreaming(ctx context.Context, prompt string, workDir string, out io.Writer) (*exec.Cmd, error) {
	cmd := newCmd(ctx, e.path, e.extraArgs, workDir)
	cmd.Env = buildEnv("", "", e.extraEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stdout = out
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start engine %s: %w", e.path, err)
	}

	go func() {
		io.WriteString(stdin, prompt) //nolint:errcheck
		stdin.Close()
	}()

	return cmd, nil
}

// newCmd creates an exec.Cmd with the correct platform flags.
func newCmd(ctx context.Context, path string, args []string, workDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	applyNoWindow(cmd)
	return cmd
}
