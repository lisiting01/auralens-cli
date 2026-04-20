package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/lisiting01/auralens-cli/internal/api"
	"github.com/lisiting01/auralens-cli/internal/output"
	"github.com/spf13/cobra"
)

var researchCmd = &cobra.Command{
	Use:   "research",
	Short: "Browse and inspect research items on the platform",
}

var researchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List research items",
	Long: `List research items on the platform.

Examples:
  auralens research list
  auralens research list --status active
  auralens research list --status active --has-result false
  auralens research list --json`,
	RunE: runResearchList,
}

var researchViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "Show full detail of a research item (with signed URLs)",
	Args:  cobra.ExactArgs(1),
	RunE:  runResearchView,
}

var researchResultCmd = &cobra.Command{
	Use:   "result <id>",
	Short: "Show the result of a research item's current round",
	Args:  cobra.ExactArgs(1),
	RunE:  runResearchResult,
}

func init() {
	rootCmd.AddCommand(researchCmd)
	researchCmd.AddCommand(researchListCmd)
	researchCmd.AddCommand(researchViewCmd)
	researchCmd.AddCommand(researchResultCmd)

	researchListCmd.Flags().String("status", "", "Filter by status: draft | active | archived")
	researchListCmd.Flags().String("has-result", "", "Filter by result presence: true | false")
	researchListCmd.Flags().Int("page", 1, "Page number")
	researchListCmd.Flags().Int("page-size", 20, "Items per page (max 50)")
}

// ── list ─────────────────────────────────────────────────────────────────────

func runResearchList(cmd *cobra.Command, args []string) error {
	cfg, err := requireAuth()
	if err != nil || cfg == nil {
		return nil
	}

	status, _ := cmd.Flags().GetString("status")
	hasResult, _ := cmd.Flags().GetString("has-result")
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")

	client := api.New(cfg.BaseURL, cfg.Name, cfg.Token)
	items, total, err := client.ListResearch(api.ListResearchParams{
		Status:    status,
		HasResult: hasResult,
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		output.Error(err.Error())
		return nil
	}

	if outputJSON {
		return output.JSON(map[string]any{
			"items": items,
			"total": total,
		})
	}

	if len(items) == 0 {
		fmt.Println(output.Faint("No research items found."))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tROUNDS\tUPDATED")
	for _, item := range items {
		statusStr := formatResearchStatus(item.Status)
		updatedAt := item.UpdatedAt
		if len(updatedAt) > 10 {
			updatedAt = updatedAt[:10]
		}
		title := item.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			item.ID, title, statusStr, item.RoundCount, updatedAt)
	}
	w.Flush()
	fmt.Printf("\n%s\n", output.Faint(fmt.Sprintf("Total: %d", total)))
	return nil
}

// ── view ─────────────────────────────────────────────────────────────────────

func runResearchView(cmd *cobra.Command, args []string) error {
	cfg, err := requireAuth()
	if err != nil || cfg == nil {
		return nil
	}

	id := args[0]
	client := api.New(cfg.BaseURL, cfg.Name, cfg.Token)
	detail, err := client.GetResearch(id)
	if err != nil {
		output.Error(err.Error())
		return nil
	}

	if outputJSON {
		return output.JSON(detail)
	}

	fmt.Printf("\n%s  %s\n", output.Bold(detail.Title), output.Faint("("+detail.ID+")"))
	fmt.Printf("Status:  %s\n", formatResearchStatus(detail.Status))
	if detail.Description != "" {
		fmt.Printf("Desc:    %s\n", detail.Description)
	}
	fmt.Printf("Rounds:  %d\n", len(detail.Rounds))
	fmt.Println()

	if detail.CurrentRound != nil {
		cr := detail.CurrentRound
		fmt.Printf("%s (round %d, %s)\n", output.Bold("Current Round"), cr.RoundNumber, formatResearchStatus(cr.Status))
		if cr.Notes != "" {
			fmt.Println()
			fmt.Println(output.Bold("Brief:"))
			fmt.Println(cr.Notes)
		}
		if len(cr.Attachments) > 0 {
			fmt.Println()
			fmt.Printf("%s (%d files):\n", output.Bold("Input Files"), len(cr.Attachments))
			for _, a := range cr.Attachments {
				fmt.Printf("  %s  %s\n", output.Cyan(a.FileName), output.Faint(a.SignedURL))
			}
		}
		if len(cr.Outputs) > 0 {
			fmt.Println()
			fmt.Printf("%s (%d files):\n", output.Bold("Output Files"), len(cr.Outputs))
			for _, o := range cr.Outputs {
				fmt.Printf("  %s  %s\n", output.Cyan(o.FileName), output.Faint(o.SignedURL))
			}
		}
		if cr.Result != "" {
			fmt.Println()
			fmt.Println(output.Bold("Result:"))
			fmt.Println(cr.Result)
		}
	}
	fmt.Println()
	return nil
}

// ── result ────────────────────────────────────────────────────────────────────

func runResearchResult(cmd *cobra.Command, args []string) error {
	cfg, err := requireAuth()
	if err != nil || cfg == nil {
		return nil
	}

	id := args[0]
	client := api.New(cfg.BaseURL, cfg.Name, cfg.Token)
	detail, err := client.GetResearch(id)
	if err != nil {
		output.Error(err.Error())
		return nil
	}

	if outputJSON {
		result := map[string]any{
			"id":    detail.ID,
			"title": detail.Title,
		}
		if detail.CurrentRound != nil {
			result["currentRound"] = map[string]any{
				"roundNumber": detail.CurrentRound.RoundNumber,
				"status":      detail.CurrentRound.Status,
				"result":      detail.CurrentRound.Result,
			}
		}
		return output.JSON(result)
	}

	fmt.Printf("\n%s — %s\n\n", output.Bold(detail.Title), detail.ID)

	if detail.CurrentRound == nil {
		fmt.Println(output.Faint("No rounds found."))
		return nil
	}

	cr := detail.CurrentRound
	fmt.Printf("Round %d (%s)\n\n", cr.RoundNumber, formatResearchStatus(cr.Status))

	if cr.Result == "" {
		fmt.Println(output.Yellow("No result submitted yet for this round."))
	} else {
		fmt.Println(output.Bold("Result:"))
		fmt.Println(cr.Result)
	}
	fmt.Println()
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func formatResearchStatus(s string) string {
	switch s {
	case "active":
		return output.Green("active")
	case "draft":
		return output.Yellow("draft")
	case "archived":
		return output.Faint("archived")
	default:
		return s
	}
}
