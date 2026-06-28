package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const githubAPI = "https://api.github.com/repos/mitchell-wallace/thenn/releases/latest"

var (
	fetchLatestVersionFunc = fetchLatestVersion
	installLatestVersionFn = installLatestVersion
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for a newer version and update in-place",
	Long: `Check the GitHub releases page for a newer version of thenn.

Prints the current and latest versions. If a newer version is available,
runs the install script immediately.`,
	Run: func(cmd *cobra.Command, args []string) {
		if version == "" || version == "dev" {
			if jsonOutput {
				printJSON(map[string]any{
					"currentVersion": "dev",
					"error":          "cannot check for updates",
				})
			} else {
				fmt.Println("Current version: dev (cannot check for updates)")
			}
			return
		}

		latest, err := fetchLatestVersionFunc()
		if err != nil {
			exit(2, "update: %v", err)
		}

		cmp, err := compareVersions(version, latest)
		if err != nil {
			exit(2, "update: %v", err)
		}

		if cmp >= 0 {
			if jsonOutput {
				printJSON(map[string]any{
					"currentVersion": version,
					"latestVersion":  latest,
					"upToDate":       true,
					"updated":        false,
				})
			} else {
				fmt.Printf("Current version: %s\nLatest version:  %s\n", version, latest)
				fmt.Println("You are up to date.")
			}
			return
		}

		if !jsonOutput {
			fmt.Printf("Current version: %s\nLatest version:  %s\n", version, latest)
		}

		if err := installLatestVersionFn(); err != nil {
			exit(2, "update: install failed: %v", err)
		}
		if jsonOutput {
			printJSON(map[string]any{
				"currentVersion": version,
				"latestVersion":  latest,
				"upToDate":       false,
				"updated":        true,
			})
		}
	},
}

func init() {
	// Kept as a hidden no-op for backwards compatibility with scripts that pass -y/--yes.
	updateCmd.Flags().BoolP("yes", "y", false, "deprecated: updates no longer prompt for confirmation")
	_ = updateCmd.Flags().MarkHidden("yes")
	rootCmd.AddCommand(updateCmd)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", githubAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return "", fmt.Errorf("GitHub API rate limit exceeded. Please try again later or set GITHUB_TOKEN environment variable")
		}
		return "", fmt.Errorf("github API returned %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("parse release: %w", err)
	}

	return strings.TrimPrefix(payload.TagName, "v"), nil
}

func installLatestVersion() error {
	var installCmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		installCmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://raw.githubusercontent.com/mitchell-wallace/thenn/main/install.ps1 | iex")
	default:
		installCmd = exec.Command("sh", "-c", "curl -fsSL https://raw.githubusercontent.com/mitchell-wallace/thenn/main/install.sh | bash")
	}
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	return installCmd.Run()
}

func compareVersions(a, b string) (int, error) {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aParts) {
			v, err := strconv.Atoi(aParts[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version %q", a)
			}
			av = v
		}
		if i < len(bParts) {
			v, err := strconv.Atoi(bParts[i])
			if err != nil {
				return 0, fmt.Errorf("invalid version %q", b)
			}
			bv = v
		}
		if av < bv {
			return -1, nil
		}
		if av > bv {
			return 1, nil
		}
	}
	return 0, nil
}
