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
func NewEngine(name, path, model string, extraArgs []string) Engine {
	switch strings.ToLower(name) {
	case "claude", "claude-code":
		return &ClaudeEngine{path: path, model: model, extraArgs: extraArgs}
	case "codex":
		return &CodexEngine{path: path, extraArgs: extraArgs}
	default:
		return &GenericEngine{path: path, extraArgs: extraArgs}
	}
}

// ── Claude Code Engine ───────────────────────────────────────────────────────

type ClaudeEngine struct {
	path      string
	model     string
	extraArgs []string
}

func (e *ClaudeEngine) Name() string { return "claude" }

func (e *ClaudeEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	args := []string{"--print", "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	args = append(args, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)

	env := os.Environ()
	if e.model != "" {
		env = append(env, "ANTHROPIC_MODEL="+e.model)
	}
	if gitBash := resolveGitBashPath(); gitBash != "" {
		env = append(env, "CLAUDE_CODE_GIT_BASH_PATH="+gitBash)
	}
	cmd.Env = env

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

	env := os.Environ()
	if e.model != "" {
		env = append(env, "ANTHROPIC_MODEL="+e.model)
	}
	if gitBash := resolveGitBashPath(); gitBash != "" {
		env = append(env, "CLAUDE_CODE_GIT_BASH_PATH="+gitBash)
	}
	cmd.Env = env

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
}

func (e *CodexEngine) Name() string { return "codex" }

func (e *CodexEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	args := append([]string{"--quiet"}, e.extraArgs...)
	cmd := newCmd(ctx, e.path, args, workDir)

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
}

func (e *GenericEngine) Name() string { return e.path }

func (e *GenericEngine) Run(ctx context.Context, prompt string, workDir string) (string, error) {
	cmd := newCmd(ctx, e.path, e.extraArgs, workDir)

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
