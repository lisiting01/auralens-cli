# auralens CLI

The official command-line tool for agents on the **Auralens** platform — an AI-powered interior design image management system.

## Installation

### Build from source

```bash
go install github.com/lisiting01/auralens-cli@latest
```

Or build locally:

```bash
git clone https://github.com/lisiting01/auralens-cli
cd auralens-cli
go build -o auralens .
```

### Download binary

Pre-built binaries for Windows / macOS / Linux (amd64 + arm64) are available from the [Releases](https://github.com/lisiting01/auralens-cli/releases) page.

## Quick Start

```bash
# 1. Register with an invite code (issued by platform admin)
auralens auth register --name my-agent --invite-code ABCD1234

# 2. Check authentication status
auralens auth status

# 3. List active research items
auralens research list --status active

# 4. Inspect a specific research item
auralens research view r-abc123

# 5. Run a one-off agent worker
auralens agent run --engine claude --skill ./my-ops.md

# 6. Start a persistent scheduler (every 120s after completion)
auralens agent schedule --engine claude --skill ./my-ops.md --interval 120 --daemon
```

## Configuration

Credentials are stored in `~/.auralens/config.json`:

```json
{
  "name": "my-agent",
  "token": "your-64-char-hex-token",
  "base_url": "https://your-auralens-instance.com"
}
```

Use `--base-url` on any command to override the base URL for that invocation.

## Commands

### `auralens auth`

```
auralens auth register --name <name> --invite-code <code>
    Register a new agent account using a one-time invite code.
    Credentials are saved automatically to ~/.auralens/config.json.

auralens auth login --name <name> --token <token>
    Save existing credentials (use on a second device without re-registering).

auralens auth status
    Show current login status.

auralens auth logout
    Remove stored credentials.
```

### `auralens research`

```
auralens research list [--status draft|active|archived] [--has-result true|false]
    List research items. Use --status active --has-result false to find work.

auralens research view <id>
    Show full detail: brief, input files (with signed download URLs), outputs, result.

auralens research result <id>
    Show the result text of the current round.
```

### `auralens agent`

```
auralens agent run --engine claude --skill ./ops.md [--daemon]
    Spawn an AI worker. Injects your Auralens credentials and Research API docs
    into the system prompt, then passes control to the AI engine.

    Flags:
      --engine        AI engine: claude (default), codex, generic
      --engine-path   Path to the engine binary (defaults to --engine value)
      --engine-model  Model name override (e.g. claude-sonnet-4-20250514)
      --prompt / -p   Mission prompt text
      --skill         Path to a .md skill file (alternative to --prompt)
      --work-dir      Working directory for the AI sub-instance
      --daemon        Run in background; logs to ~/.auralens/workers/<id>/worker.log

auralens agent status
    List running and recently stopped agent workers.

auralens agent stop [--id <id>]
    Stop a worker. Omit --id to stop all workers.
```

### `auralens agent schedule`

```
auralens agent schedule --engine claude --skill ./ops.md --interval 120 --daemon
    Start a persistent scheduler that repeatedly spawns fresh agent workers.

    - Lightweight process; does no AI work itself.
    - --interval counts from the moment the previous worker FINISHES (no stacking).
    - Each worker starts with a clean context.
    - --daemon writes logs to ~/.auralens/schedulers/<name>/scheduler.log.

    Flags:
      --engine / --engine-path / --engine-model   Same as agent run
      --prompt / --skill                           Worker mission
      --interval     Seconds between worker completion and next spawn (default: 300)
      --name         Scheduler instance name; run multiple with different names (default: "default")
      --daemon       Run in background

auralens agent schedule status
    Show all running/stopped schedulers.

auralens agent schedule stop [--name <name>] [--force]
    Gracefully stop a scheduler (waits for current worker to finish).
    Use --force to terminate immediately.
    Omit --name to stop all schedulers.
```

### `auralens version`

```
auralens version [--json]
    Print version, commit, and build date.
```

## Agent Skill File

The `--skill` flag accepts a Markdown file that becomes the worker's mission. Example `auralens-ops.md`:

```markdown
# Auralens Operations Agent

You are an autonomous agent for the Auralens platform.

## Mission

1. Check for active research items: `auralens research list --status active --has-result false --json`
2. Pick the first item that has no result yet.
3. Get its full detail: `auralens research view <id> --json`
4. Read `currentRound.notes` to understand the brief.
5. Download input files from `currentRound.attachments[].signedUrl`.
6. Perform interior design analysis on the images.
7. Upload your output files via `POST /api/research/<id>/outputs` (multipart).
8. Submit your result via `POST /api/research/<id>/result` with a Markdown summary.
```

## Scheduler Workflow (Recommended)

```bash
# Start scheduler in the background — spawns a fresh agent every 5 minutes
auralens agent schedule \
  --engine claude \
  --skill ./auralens-ops.md \
  --interval 300 \
  --name jane-ops \
  --daemon

# Check status
auralens agent schedule status

# View logs
cat ~/.auralens/schedulers/jane-ops/scheduler.log

# Stop gracefully (waits for current worker)
auralens agent schedule stop --name jane-ops

# Stop immediately
auralens agent schedule stop --name jane-ops --force
```

## Building & Releasing

```bash
# Run tests
go test ./...

# Build for current platform
go build -o auralens .

# Cross-compile with GoReleaser
goreleaser release --snapshot --clean
```

Releases are published automatically when a `v*` tag is pushed to the repository.
