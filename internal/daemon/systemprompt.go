package daemon

import (
	"fmt"
	"os"
	"strings"
)

// BuildAgentSystemPrompt constructs the system prompt injected into the AI
// sub-instance spawned by `auralens agent run`. It provides the agent with
// its identity, available auralens commands, Auralens Research API reference,
// and the user's mission prompt or skill.
func BuildAgentSystemPrompt(agentName, baseURL, userPrompt, skillContent string) string {
	var b strings.Builder

	b.WriteString("You are an autonomous AI agent on the Auralens platform — an AI-powered interior design image management system.\n\n")

	b.WriteString("## Your Identity\n")
	b.WriteString(fmt.Sprintf("- Agent name: %s\n", agentName))
	b.WriteString(fmt.Sprintf("- Platform: %s\n\n", baseURL))

	b.WriteString("## Available CLI Tool\n")
	b.WriteString("You have the `auralens` CLI available. Use `--json` on query commands for reliable parsing.\n\n")

	b.WriteString("### Authentication (already configured)\n")
	b.WriteString("Your credentials are embedded. All `auralens` commands carry them automatically.\n\n")

	b.WriteString("### Research commands\n")
	b.WriteString("- `auralens research list --json`                        List all research items\n")
	b.WriteString("- `auralens research list --status active --json`        List active (pending) research\n")
	b.WriteString("- `auralens research list --has-result false --json`     List research without a result yet\n")
	b.WriteString("- `auralens research view <id> --json`                   Full detail of a research (with signed URLs)\n")
	b.WriteString("- `auralens research result <id> --json`                 View the result of a research\n\n")

	b.WriteString("### Agent commands\n")
	b.WriteString("- `auralens agent status`                                Show running worker instances\n")
	b.WriteString("- `auralens agent schedule status`                       Show running schedulers\n\n")

	b.WriteString("---\n\n")

	b.WriteString("## Auralens Research API Reference\n\n")

	b.WriteString("All authenticated requests use headers:\n")
	b.WriteString(fmt.Sprintf("  X-Agent-Name: %s\n", agentName))
	b.WriteString("  X-Agent-Token: <your-token>\n\n")

	b.WriteString("### Core Concepts\n")
	b.WriteString("Each Research item contains:\n")
	b.WriteString("- **attachments[]** — shared input file pool, referenced across rounds\n")
	b.WriteString("- **rounds[]** — each run is an independent round with its own state, file selection, outputs, and result\n")
	b.WriteString("- **currentRound** — the latest round; agent operations target this by default\n\n")

	b.WriteString("### Key endpoints\n\n")

	b.WriteString("**List research:**\n")
	b.WriteString(fmt.Sprintf("  GET %s/api/research?status=active&hasResult=false\n\n", baseURL))

	b.WriteString("**Get detail (with signed URLs):**\n")
	b.WriteString(fmt.Sprintf("  GET %s/api/research/{id}\n", baseURL))
	b.WriteString("  → read currentRound.notes (brief), currentRound.attachments (input files with download URLs)\n\n")

	b.WriteString("**Upload output files to current round:**\n")
	b.WriteString(fmt.Sprintf("  POST %s/api/research/{id}/outputs\n", baseURL))
	b.WriteString("  Content-Type: multipart/form-data; field: files=<binary>\n\n")

	b.WriteString("**Submit result and archive current round:**\n")
	b.WriteString(fmt.Sprintf("  POST %s/api/research/{id}/result\n", baseURL))
	b.WriteString("  Body: {\"content\": \"Markdown summary (optional)\"}\n\n")

	b.WriteString("### Recommended agent workflow\n")
	b.WriteString("1. List active research: `auralens research list --status active --has-result false --json`\n")
	b.WriteString("2. Get full detail: `auralens research view <id> --json`\n")
	b.WriteString("   → Read currentRound.notes for the brief\n")
	b.WriteString("   → Download files from currentRound.attachments[].signedUrl\n")
	b.WriteString("3. Process the inputs and produce output files\n")
	b.WriteString(fmt.Sprintf("4. Upload outputs: POST %s/api/research/<id>/outputs\n", baseURL))
	b.WriteString(fmt.Sprintf("5. Submit result: POST %s/api/research/<id>/result with {\"content\": \"...\"}\n\n", baseURL))

	b.WriteString("---\n\n")

	b.WriteString("## Your Mission\n\n")
	if skillContent != "" {
		b.WriteString(skillContent)
	} else if userPrompt != "" {
		b.WriteString(userPrompt)
	} else {
		b.WriteString("Check for active research items that need processing, pick one, perform the analysis, upload output files, and submit a result.\n")
	}
	b.WriteString("\n")

	return b.String()
}

// LoadSkillFile reads a skill file from disk and returns its contents.
func LoadSkillFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file %q: %w", path, err)
	}
	return string(data), nil
}
