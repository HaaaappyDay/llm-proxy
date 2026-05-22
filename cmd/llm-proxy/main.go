package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lotus/llm-proxy/internal/app"
	"github.com/lotus/llm-proxy/internal/auth"
	"github.com/lotus/llm-proxy/internal/config"
	"github.com/lotus/llm-proxy/internal/proxy"
	"github.com/lotus/llm-proxy/internal/server"
	"github.com/spf13/cobra"
)

func main() {
	cfg := config.Default()
	root := &cobra.Command{
		Use:   "llm-proxy",
		Short: "Local OAuth gateway for Codex and GitHub Copilot",
	}

	root.PersistentFlags().StringVar(&cfg.ListenHost, "host", cfg.ListenHost, "listen host")
	root.PersistentFlags().StringVar(&cfg.ListenPort, "port", cfg.ListenPort, "listen port")
	root.PersistentFlags().StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory (~/.llm-proxy)")

	root.AddCommand(serveCmd(cfg))
	root.AddCommand(loginCmd(cfg))
	root.AddCommand(doctorCmd(cfg))

	if err := root.Execute(); err != nil {
		os.Exit(1)
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
				Addr:    cfg.ListenAddr(),
				Handler: router,
			}
			fmt.Printf("llm-proxy listening on %s\n", cfg.BaseURL())
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

func loginCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [codex|copilot]",
		Short: "OAuth device login and create API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := args[0]
			application := app.New(cfg)
			switch provider {
			case "codex":
				return runLogin(application, auth.ProviderCodexOAuth, application.Codex.StartDeviceFlow, application.Codex.PollForToken, application.Codex.DefaultAccountID)
			case "copilot":
				return runLogin(application, auth.ProviderGitHubCopilot, application.Copilot.StartDeviceFlow, application.Copilot.PollForToken, application.Copilot.DefaultAccountID)
			default:
				return fmt.Errorf("unknown provider %q (use codex or copilot)", provider)
			}
		},
	}
	return cmd
}

type pollFn func(deviceCode string) (*auth.Account, error)

func runLogin(
	application *app.App,
	provider string,
	start func() (*auth.DeviceCodeResponse, error),
	poll pollFn,
	defaultAccount func() string,
) error {
	device, err := start()
	if err != nil {
		return err
	}
	fmt.Printf("Open: %s\n", device.VerificationURI)
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
	fmt.Printf("export LLM_PROXY_API_KEY=%s\n", result.Plaintext)
	fmt.Printf("export ANTHROPIC_BASE_URL=%s\n", application.Config.BaseURL())
	fmt.Printf("export ANTHROPIC_AUTH_TOKEN=%s\n", result.Plaintext)
	return nil
}

func doctorCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues := proxy.Doctor(cfg.DataDir)
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
