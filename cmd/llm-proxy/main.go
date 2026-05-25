package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/HaaapyDay/llm-proxy/internal/app"
	"github.com/HaaapyDay/llm-proxy/internal/auth"
	"github.com/HaaapyDay/llm-proxy/internal/config"
	"github.com/HaaapyDay/llm-proxy/internal/proxy"
	"github.com/HaaapyDay/llm-proxy/internal/server"
	"github.com/spf13/cobra"
)

const apiKeySaveWarning = "Save this API key now. It is shown only once; if lost, create a new key with `llm-proxy keys create codex` or `llm-proxy keys create copilot`."

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cfg := config.Default()
	root := &cobra.Command{
		Use:     "llm-proxy",
		Short:   "Local OAuth gateway for Codex and GitHub Copilot",
		Version: versionString(),
	}

	root.PersistentFlags().StringVar(&cfg.ListenHost, "host", cfg.ListenHost, "listen host")
	root.PersistentFlags().StringVar(&cfg.ListenPort, "port", cfg.ListenPort, "listen port")
	root.PersistentFlags().StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory (~/.llm-proxy)")

	root.AddCommand(serveCmd(cfg))
	root.AddCommand(loginCmd(cfg))
	root.AddCommand(keysCmd(cfg))
	root.AddCommand(doctorCmd(cfg))
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionString() string {
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("llm-proxy %s\n", versionString())
		},
	}
}

func keysCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage local API keys",
	}
	cmd.AddCommand(keysListCmd(cfg))
	cmd.AddCommand(keysCreateCmd(cfg))
	cmd.AddCommand(keysDeleteCmd(cfg))
	return cmd
}

func keysListCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active local API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			application := app.New(cfg)
			keys, err := application.APIKeys.ListActive()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tPROVIDER\tACCOUNT\tLABEL\tKEY\tCREATED")
			for _, key := range keys {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					key.ID,
					key.Provider,
					key.AccountID,
					key.Label,
					key.KeyPreview,
					time.Unix(key.CreatedAt, 0).Format(time.RFC3339),
				)
			}
			return w.Flush()
		},
	}
}

func keysCreateCmd(cfg *config.Config) *cobra.Command {
	label := "default"
	cmd := &cobra.Command{
		Use:   "create [codex|copilot]",
		Short: "Create a local API key for a logged-in provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			application := app.New(cfg)
			provider, accountID, err := providerAndAccountID(application, args[0])
			if err != nil {
				return err
			}
			result, err := application.APIKeys.Create(auth.CreateKeyInput{
				Label:     label,
				Provider:  provider,
				AccountID: accountID,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Created API key %s for %s (%s)\n", result.Record.ID, args[0], accountID)
			writeAPIKeyEnvironment(os.Stdout, application.Config.BaseURL(), result.Plaintext)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", label, "key label")
	return cmd
}

func keysDeleteCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "delete KEY_ID",
		Short: "Revoke a local API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			application := app.New(cfg)
			if err := application.APIKeys.Delete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Revoked API key %s\n", args[0])
			return nil
		},
	}
}

func providerAndAccountID(application *app.App, providerArg string) (string, string, error) {
	switch providerArg {
	case "codex":
		accountID := application.Codex.DefaultAccountID()
		if accountID == "" {
			accounts := application.Codex.ListAccounts()
			if len(accounts) == 1 {
				accountID = accounts[0].ID
			}
		}
		if accountID == "" {
			return "", "", fmt.Errorf("no codex account found; run `llm-proxy login codex` first")
		}
		return auth.ProviderCodexOAuth, accountID, nil
	case "copilot":
		accountID := application.Copilot.DefaultAccountID()
		if accountID == "" {
			accounts := application.Copilot.ListAccounts()
			if len(accounts) == 1 {
				accountID = accounts[0].ID
			}
		}
		if accountID == "" {
			return "", "", fmt.Errorf("no copilot account found; run `llm-proxy login copilot` first")
		}
		return auth.ProviderGitHubCopilot, accountID, nil
	default:
		return "", "", fmt.Errorf("unknown provider %q (use codex or copilot)", providerArg)
	}
}

func serveCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			application := app.New(cfg)
			router := server.NewRouter(application)
			srv := &http.Server{
				Addr:              cfg.ListenAddr(),
				Handler:           router,
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       60 * time.Second,
			}
			fmt.Printf("llm-proxy listening on %s\n", cfg.BaseURL())
			if warning := publicListenWarning(cfg.ListenHost); warning != "" {
				fmt.Fprintln(os.Stderr, warning)
			}
			if cfg.Debug {
				fmt.Fprintln(os.Stderr, "llm-proxy debug logging enabled")
			}
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Fprintf(os.Stderr, "server error: %v\n", err)
					os.Exit(1)
				}
			}()
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
			<-stop
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(ctx)
		},
	}
}

func publicListenWarning(host string) string {
	if host == config.DefaultListenHost || host == "localhost" || host == "::1" {
		return ""
	}
	return fmt.Sprintf("WARNING: llm-proxy is listening on %s; do not expose this service to untrusted networks", host)
}

func loginCmd(cfg *config.Config) *cobra.Command {
	noBrowser := false
	cmd := &cobra.Command{
		Use:   "login [codex|copilot]",
		Short: "OAuth device login and create API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			application := app.New(cfg)
			switch provider {
			case "codex":
				return runLogin(application, auth.ProviderCodexOAuth, application.Codex.StartDeviceFlow, application.Codex.PollForToken, application.Codex.DefaultAccountID, !noBrowser)
			case "copilot":
				return runLogin(application, auth.ProviderGitHubCopilot, application.Copilot.StartDeviceFlow, application.Copilot.PollForToken, application.Copilot.DefaultAccountID, !noBrowser)
			default:
				return fmt.Errorf("unknown provider %q (use codex or copilot)", provider)
			}
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the verification URL instead of opening a browser")
	return cmd
}

type pollFn func(deviceCode string) (*auth.Account, error)

func runLogin(
	application *app.App,
	provider string,
	start func() (*auth.DeviceCodeResponse, error),
	poll pollFn,
	defaultAccount func() string,
	openVerificationURL bool,
) error {
	device, err := start()
	if err != nil {
		return err
	}
	if openVerificationURL {
		if err := openBrowser(device.VerificationURI); err == nil {
			fmt.Printf("Opened browser: %s\n", device.VerificationURI)
		} else {
			fmt.Printf("Open: %s\n", device.VerificationURI)
		}
	} else {
		fmt.Printf("Open: %s\n", device.VerificationURI)
	}
	fmt.Printf("Code: %s\n", device.UserCode)
	fmt.Println("Waiting for authorization...")

	interval := time.Duration(device.Interval) * time.Second
	if interval < 3*time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)

	var account *auth.Account
	for time.Now().Before(deadline) {
		acc, err := poll(device.DeviceCode)
		if err == auth.ErrAuthPending {
			time.Sleep(interval)
			continue
		}
		if err != nil {
			return err
		}
		account = acc
		break
	}
	if account == nil {
		return fmt.Errorf("login timed out")
	}

	accountID := account.ID
	if accountID == "" {
		accountID = defaultAccount()
	}
	result, err := application.APIKeys.Create(auth.CreateKeyInput{
		Label:     "default",
		Provider:  provider,
		AccountID: accountID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Logged in as %s (%s)\n", account.Login, account.ID)
	writeAPIKeyEnvironment(os.Stdout, application.Config.BaseURL(), result.Plaintext)
	return nil
}

func writeAPIKeyEnvironment(w io.Writer, baseURL, plaintext string) {
	fmt.Fprintf(w, "export LLM_PROXY_API_KEY=%s\n", plaintext)
	fmt.Fprintf(w, "export ANTHROPIC_BASE_URL=%s\n", baseURL)
	fmt.Fprintf(w, "export ANTHROPIC_AUTH_TOKEN=%s\n", plaintext)
	fmt.Fprintf(w, "export OPENAI_BASE_URL=%s/v1\n", baseURL)
	fmt.Fprintf(w, "export OPENAI_API_KEY=%s\n", plaintext)
	fmt.Fprintln(w, apiKeySaveWarning)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func doctorCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues := proxy.Doctor(cfg)
			fmt.Printf("Data directory: %s\n", cfg.DataDir)
			fmt.Printf("Listen address: %s\n", cfg.ListenAddr())
			if len(issues) == 0 {
				fmt.Println("No issues found.")
				return nil
			}
			for _, issue := range issues {
				fmt.Printf("- %s\n", issue)
			}
			return nil
		},
	}
}
