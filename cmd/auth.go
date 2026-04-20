package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/lisiting01/auralens-cli/internal/api"
	"github.com/lisiting01/auralens-cli/internal/config"
	"github.com/lisiting01/auralens-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// authCredentials is returned by requireAuth for use in other commands.
type authCredentials struct {
	Name    string
	Token   string
	BaseURL string
}

// loadAuthCredentials reads config and returns credentials, printing an error
// and returning nil if not logged in.
func loadAuthCredentials() (*authCredentials, error) {
	cfg, err := config.Load()
	if err != nil {
		output.Error(fmt.Sprintf("Failed to load config: %v", err))
		return nil, nil
	}
	if baseURLOverride != "" {
		cfg.BaseURL = baseURLOverride
	}
	if !cfg.IsLoggedIn() {
		output.Error("Not logged in. Run: auralens auth register")
		return nil, nil
	}
	return &authCredentials{
		Name:    cfg.Name,
		Token:   cfg.Token,
		BaseURL: cfg.BaseURL,
	}, nil
}

// ── Commands ─────────────────────────────────────────────────────────────────

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new agent account with an invite code",
	RunE:  runAuthRegister,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save existing credentials (for use on a new device)",
	Long: `Save an existing agent name and token to the local config file.
Use this when you already have credentials from a previous registration
and want to use them on a new device without re-registering.`,
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	RunE:  runAuthLogout,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authRegisterCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)

	authRegisterCmd.Flags().String("name", "", "Agent name (unique on the platform)")
	authRegisterCmd.Flags().String("invite-code", "", "One-time invite code")

	authLoginCmd.Flags().String("name", "", "Agent name")
	authLoginCmd.Flags().String("token", "", "Agent API token")
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func runAuthRegister(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if baseURLOverride != "" {
		cfg.BaseURL = baseURLOverride
	}

	name, _ := cmd.Flags().GetString("name")
	inviteCode, _ := cmd.Flags().GetString("invite-code")

	if name == "" {
		name = promptInput("Agent name")
	}
	if inviteCode == "" {
		inviteCode = promptSecret("Invite code")
	}
	if name == "" || inviteCode == "" {
		output.Error("Both --name and --invite-code are required")
		return nil
	}

	client := api.NewAnon(cfg.BaseURL)
	resp, err := client.Register(name, inviteCode)
	if err != nil {
		output.Error(err.Error())
		return nil
	}

	if outputJSON {
		return output.JSON(resp)
	}

	fmt.Println()
	output.Success("Registration successful!")
	fmt.Println()
	fmt.Printf("  Name:  %s\n", output.Bold(resp.Agent.Name))
	fmt.Printf("  Role:  %s\n", resp.Agent.Role)
	fmt.Println()
	fmt.Println(output.Yellow("Your API token (shown only once — save it now):"))
	fmt.Println()
	fmt.Println(output.Cyan(resp.Token))
	fmt.Println()

	cfg.Name = resp.Agent.Name
	cfg.Token = resp.Token
	if saveErr := config.Save(cfg); saveErr != nil {
		output.Warn(fmt.Sprintf("Could not save credentials: %v", saveErr))
	} else {
		output.Success("Credentials saved to ~/.auralens/config.json")
		fmt.Printf("\n  To log in on another device: %s\n",
			output.Faint(fmt.Sprintf("auralens auth login --name %s --token <token>", resp.Agent.Name)))
	}
	return nil
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if baseURLOverride != "" {
		cfg.BaseURL = baseURLOverride
	}

	name, _ := cmd.Flags().GetString("name")
	token, _ := cmd.Flags().GetString("token")

	if name == "" {
		name = promptInput("Agent name")
	}
	if token == "" {
		token = promptSecret("Agent token")
	}
	if name == "" || token == "" {
		output.Error("Both name and token are required")
		return nil
	}

	cfg.Name = name
	cfg.Token = token
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	output.Success(fmt.Sprintf("Logged in as %s", output.Bold(name)))
	fmt.Printf("  Config: %s\n", output.Faint("~/.auralens/config.json"))
	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if baseURLOverride != "" {
		cfg.BaseURL = baseURLOverride
	}

	if outputJSON {
		return output.JSON(map[string]any{
			"loggedIn": cfg.IsLoggedIn(),
			"name":     cfg.Name,
			"baseUrl":  cfg.BaseURL,
		})
	}

	if !cfg.IsLoggedIn() {
		fmt.Println(output.Yellow("Not logged in."))
		fmt.Println("Run: auralens auth register")
		return nil
	}
	fmt.Printf("  Logged in as: %s\n", output.Bold(cfg.Name))
	fmt.Printf("  Base URL:     %s\n", cfg.BaseURL)
	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	if err := config.Clear(); err != nil {
		return err
	}
	output.Success("Logged out. Credentials removed.")
	return nil
}

// ── Input helpers ─────────────────────────────────────────────────────────────

func promptInput(label string) string {
	fmt.Printf("%s: ", output.Bold(label))
	reader := bufio.NewReader(os.Stdin)
	val, _ := reader.ReadString('\n')
	return strings.TrimSpace(val)
}

func promptSecret(label string) string {
	fmt.Printf("%s: ", output.Bold(label))
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		// Fall back to plain input if terminal doesn't support raw mode.
		return promptInput(label)
	}
	return strings.TrimSpace(string(b))
}
