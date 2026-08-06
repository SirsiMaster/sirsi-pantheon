package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/localrouter"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

// SNE owns local inference on the portfolio-standard OpenAI-compatible socket.
// The retired Python mlx_lm broker used 8765.
const gemmaServerDefaultPort = 8477

var (
	gemmaServeStop        bool
	gemmaServeStatusFlag  bool
	gemmaServeForeground  bool
	gemmaServePort        int
	gemmaServeConcurrency int
)

var gemmaServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start or inspect the native SNE local-inference service",
	Long: `Uses launchd to run one reboot-durable SNE process on the local machine.
Every Pantheon client shares that Go service; this command never starts a Python
inference process or a second model broker.

  sirsi gemma serve
  sirsi gemma serve --status
  sirsi gemma serve --stop`,
	RunE: runGemmaServe,
}

func init() {
	gemmaServeCmd.Flags().BoolVar(&gemmaServeStop, "stop", false, "Stop the local SNE service")
	gemmaServeCmd.Flags().BoolVar(&gemmaServeStatusFlag, "status", false, "Show whether SNE is ready")
	// Retained for CLI compatibility while launchd owns these choices.
	gemmaServeCmd.Flags().BoolVar(&gemmaServeForeground, "foreground", false, "Deprecated: launchd owns the SNE process")
	gemmaServeCmd.Flags().IntVar(&gemmaServePort, "port", gemmaServerDefaultPort, "Local SNE port")
	gemmaServeCmd.Flags().IntVar(&gemmaServeConcurrency, "concurrency", 0, "Deprecated: SNE profile owns concurrency")
	gemmaCmd.AddCommand(gemmaServeCmd)
}

func gemmaPidPath(home string) string       { return filepath.Join(home, ".sirsi/gemma-server.pid") }
func gemmaPortPath(home string) string      { return filepath.Join(home, ".sirsi/gemma-server.port") }
func gemmaServerLogPath(home string) string { return filepath.Join(home, ".sirsi/sne-server.log") }

func runGemmaServe(cmd *cobra.Command, args []string) error {
	home, _ := os.UserHomeDir()
	switch {
	case gemmaServeStop:
		return gemmaServerStop(home)
	case gemmaServeStatusFlag:
		if base := gemmaServerBase(home); base != "" {
			fmt.Printf("SNE local inference: WARM at %s.\n", base)
		} else {
			fmt.Println("SNE local inference: unavailable. Start it: sirsi gemma serve")
		}
		return nil
	default:
		if !setup.GemmaBrokerInstalled() {
			return fmt.Errorf("SNE is not installed — run `sirsi setup`; Python inference fallback is retired")
		}
		kickCtx, cancelKick := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelKick()
		label := fmt.Sprintf("gui/%d/%s", os.Getuid(), setup.GemmaBrokerLabel)
		if err := exec.CommandContext(kickCtx, "launchctl", "kickstart", label).Run(); err != nil {
			return fmt.Errorf("starting SNE through launchd: %w", err)
		}
		return gemmaAwaitWarm(home)
	}
}

func gemmaAwaitWarm(home string) error {
	base := fmt.Sprintf("http://127.0.0.1:%d", gemmaServePort)
	for i := 0; i < 45; i++ {
		time.Sleep(2 * time.Second)
		if gemmaServerPing(base) {
			fmt.Printf("✓ SNE is WARM at %s.\n", base)
			return nil
		}
	}
	return fmt.Errorf("SNE didn't become healthy in ~90s — check %s", gemmaServerLogPath(home))
}

func gemmaServerStop(home string) error {
	b, err := os.ReadFile(gemmaPidPath(home))
	if err != nil {
		fmt.Println("No local inference process recorded.")
		return nil
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 {
		_ = guard.HapiUnregisterGoverned(pid)
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	_ = os.Remove(gemmaPidPath(home))
	_ = os.Remove(gemmaPortPath(home))
	fmt.Printf("Stopped SNE local inference (pid %d).\n", pid)
	if setup.GemmaBrokerInstalled() {
		fmt.Printf("Note: launchd will restore it. To keep it off: launchctl bootout gui/%d/%s\n", os.Getuid(), setup.GemmaBrokerLabel)
	}
	return nil
}

func gemmaServerBase(home string) string {
	b, err := os.ReadFile(gemmaPortPath(home))
	if err != nil {
		return ""
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || port == 0 {
		return ""
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if gemmaServerPing(base) {
		return base
	}
	return ""
}

func gemmaServerPing(base string) bool {
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(base + "/v1/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func gemmaWarmComplete(base, model, prompt string, maxTokens int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": localrouter.SystemPrompt()},
			{"role": "user", "content": prompt},
		},
		"max_tokens":  maxTokens,
		"temperature": 0,
	})
	cl := &http.Client{Timeout: 5 * time.Minute}
	resp, err := cl.Post(base+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("SNE returned no choices")
	}
	return gemmaClean(out.Choices[0].Message.Content), nil
}
