package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/spf13/cobra"
	openai "github.com/sashabaranov/go-openai"
)

var statusProfileName string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of a profile",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := resolveConfigPath(cfgFile)
		if err != nil {
			return err
		}
		manager := config.NewConfigManager(configPath)

		profile, err := manager.GetAgentProfileConfig(statusProfileName)
		if err != nil {
			return fmt.Errorf("profile %q not found: %w", statusProfileName, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Profile:   %s\n", statusProfileName)
		fmt.Fprintf(cmd.OutOrStdout(), "Provider:  %s\n", profile.Provider)
		fmt.Fprintf(cmd.OutOrStdout(), "Model:     %s\n", profile.Model)
		fmt.Fprintf(cmd.OutOrStdout(), "Workspace: %s\n", profile.Workspace)

		sessionIDs, latestSession, tokenCount, err := loadCurrentSessionStats(profile.Workspace)
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Session:   (unable to read sessions: %v)\n", err)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Sessions:  %d active\n", len(sessionIDs))
		if latestSession != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Current:   %s\n", latestSession)
			fmt.Fprintf(cmd.OutOrStdout(), "Tokens:    ~%d (estimated)\n", tokenCount)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Current:   (no active session)\n")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVarP(&statusProfileName, "profile", "p", "default", "profile name to show status for")
}

// loadCurrentSessionStats scans the workspace sessions directory and returns:
// all session IDs, the ID of the most-recently-modified session, and an estimated token count.
func loadCurrentSessionStats(workspace string) ([]string, string, int, error) {
	sessionsDir := filepath.Join(workspace, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if os.IsNotExist(err) {
		return nil, "", 0, nil
	}
	if err != nil {
		return nil, "", 0, err
	}

	sessionIDs := make([]string, 0)
	var latestSession string
	var latestModTime int64

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		info, err := entry.Info()
		if err != nil {
			continue
		}
		sessionIDs = append(sessionIDs, sessionID)
		if info.ModTime().Unix() > latestModTime {
			latestModTime = info.ModTime().Unix()
			latestSession = sessionID
		}
	}

	if latestSession == "" {
		return sessionIDs, "", 0, nil
	}

	tokens, err := estimateSessionTokens(sessionsDir, latestSession)
	if err != nil {
		return sessionIDs, latestSession, 0, nil
	}
	return sessionIDs, latestSession, tokens, nil
}

// estimateSessionTokens loads a session snapshot (+ WAL) and estimates token usage
// using the rough heuristic of 1 token ≈ 4 characters of text.
func estimateSessionTokens(sessionsDir string, sessionID string) (int, error) {
	snapshotPath := filepath.Join(sessionsDir, sessionID+".json")
	content, err := os.ReadFile(snapshotPath)
	if err != nil {
		return 0, err
	}
	var snap session.SessionFile
	if err := json.Unmarshal(content, &snap); err != nil {
		return 0, err
	}

	// Replay WAL if present.
	walPath := snapshotPath + ".wal"
	if walContent, err := os.ReadFile(walPath); err == nil {
		for _, line := range strings.Split(string(walContent), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry struct {
				Revision uint64                         `json:"revision"`
				Op       string                         `json:"op"`
				Messages []openai.ChatCompletionMessage `json:"messages,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if entry.Op == "append_messages" && entry.Revision > snap.Revision {
				snap.Messages = append(snap.Messages, entry.Messages...)
				snap.Revision = entry.Revision
			}
		}
	}

	total := 0
	for _, msg := range snap.Messages {
		total += estimateMessageTokens(msg)
	}
	return total, nil
}

// estimateMessageTokens uses 1 token ≈ 4 chars as a rough approximation.
func estimateMessageTokens(msg openai.ChatCompletionMessage) int {
	chars := len(msg.Content) + len(msg.Role)
	for _, tc := range msg.ToolCalls {
		chars += len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	if msg.FunctionCall != nil {
		chars += len(msg.FunctionCall.Name) + len(msg.FunctionCall.Arguments)
	}
	tokens := chars / 4
	if tokens == 0 && chars > 0 {
		tokens = 1
	}
	return tokens
}
