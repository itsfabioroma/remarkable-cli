// Package update provides self-update and new-version awareness for the
// remarkable CLI. It checks GitHub for the latest commit on main, caches
// the result for a day, notifies the user (non-intrusively) when they're
// behind, and rebuilds the binary in place on demand.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// GitHub repo the CLI lives in. Matches the module path in go.mod.
const (
	repoOwner = "itsfabioroma"
	repoName  = "remarkable-cli"
	modulePath = "github.com/itsfabioroma/remarkable-cli"
	// how often we poll GitHub (silent background check)
	checkInterval = 24 * time.Hour
)

// Cache is what we persist to disk between runs
type Cache struct {
	LastCheckUnix int64  `json:"last_check_unix"`
	LatestSHA     string `json:"latest_sha"`
	LatestDate    string `json:"latest_date"`
	// LatestTag is the semver tag of the latest GitHub Release, if any.
	// Empty string means we fell back to commits/main (no releases yet).
	LatestTag string `json:"latest_tag,omitempty"`
	// ShownForSHA suppresses repeat banners for the same upgrade
	ShownForSHA string `json:"shown_for_sha,omitempty"`
}

// cachePath is the on-disk location of the update cache
func cachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "remarkable-cli", "update-check.json")
}

// loadCache returns the last cached check (empty struct on miss)
func loadCache() Cache {
	var c Cache
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

// saveCache writes the cache back to disk (best effort)
func saveCache(c Cache) {
	data, _ := json.Marshal(c)
	_ = os.MkdirAll(filepath.Dir(cachePath()), 0700)
	_ = os.WriteFile(cachePath(), data, 0600)
}

// currentSHA returns the vcs.revision embedded at build time, or "" if dev build
func currentSHA() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}

// currentBuildTime returns the vcs.time embedded at build time, or zero time
func currentBuildTime() time.Time {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return time.Time{}
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.time" {
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// shortSHA returns the first 7 chars for display
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// latestRelease fetches the latest GitHub Release. Returns the tag name,
// the commit SHA the release was built from, and the publish date.
// Returns (empty, empty, empty, nil) when no release exists (404) —
// callers should then fall back to latestCommit.
func latestRelease(ctx context.Context) (tag, sha, date string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "remarkable-cli-update-check")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", "", "", nil // no releases yet — caller should fall back
	}
	if resp.StatusCode != 200 {
		return "", "", "", fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var body struct {
		TagName     string `json:"tag_name"`
		TargetSHA   string `json:"target_commitish"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", "", err
	}
	return body.TagName, body.TargetSHA, body.PublishedAt, nil
}

// latestCommit fetches the latest commit SHA on main via GitHub's public API.
// Used as a fallback when no GitHub Release exists. No auth needed for public
// repos; rate-limited to 60 req/hr per IP.
func latestCommit(ctx context.Context) (sha, date string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/main", repoOwner, repoName)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "remarkable-cli-update-check")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var body struct {
		SHA    string `json:"sha"`
		Commit struct {
			Author struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	return body.SHA, body.Commit.Author.Date, nil
}

// fetchLatest tries releases first, falls back to main-branch commit.
// Returns (tag, sha, date). tag is empty when falling back.
func fetchLatest(ctx context.Context) (tag, sha, date string, err error) {
	tag, sha, date, err = latestRelease(ctx)
	if err != nil {
		return "", "", "", err
	}
	if tag != "" {
		return tag, sha, date, nil
	}
	// no release published yet — fall back to main-branch commit
	sha, date, err = latestCommit(ctx)
	return "", sha, date, err
}

// BackgroundCheck spawns a detached goroutine that refreshes the cache if
// it's older than checkInterval. Returns immediately — never blocks the CLI.
// Safe to call once per invocation from the main command path.
var bgOnce sync.Once

func BackgroundCheck() {
	bgOnce.Do(func() {
		go func() {
			defer func() { _ = recover() }() // never crash the CLI for an update check
			c := loadCache()
			age := time.Since(time.Unix(c.LastCheckUnix, 0))
			if c.LastCheckUnix > 0 && age < checkInterval {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tag, sha, date, err := fetchLatest(ctx)
			if err != nil {
				return
			}
			c.LastCheckUnix = time.Now().Unix()
			c.LatestTag = tag
			c.LatestSHA = sha
			c.LatestDate = date
			saveCache(c)
		}()
	})
}

// Notify prints a one-line banner to stderr if a newer commit exists on
// GitHub than the one this binary was built from. "Newer" is determined by
// commit timestamp, not just SHA — so locally-ahead builds don't trigger a
// bogus "downgrade" banner. Silent when:
//   - build has no VCS info (dev build)
//   - cache is empty (first run, background check hasn't completed)
//   - cached latest is not strictly newer than current build time
//   - the upgrade has already been shown for this SHA (no nagging)
//   - stderr is not a terminal OR jsonMode is true (don't pollute agent output)
func Notify(stderrIsTerminal, jsonMode bool) {
	if !stderrIsTerminal || jsonMode {
		return
	}
	cur := currentSHA()
	if cur == "" {
		return
	}
	c := loadCache()
	if c.LatestSHA == "" || c.LatestSHA == cur || c.ShownForSHA == c.LatestSHA {
		return
	}
	// only notify if remote is strictly NEWER than our build (by date)
	if !isRemoteNewer(c.LatestDate) {
		return
	}

	fmt.Fprintf(os.Stderr, "\x1b[33m▲\x1b[0m update available: %s → %s  (run `remarkable update`)\n",
		shortSHA(cur), shortSHA(c.LatestSHA))

	// mark as shown so we don't nag on every invocation
	c.ShownForSHA = c.LatestSHA
	saveCache(c)
}

// SelfUpdate rebuilds the binary to match the latest main. Strategy:
//  1. Prefer `git pull + go build` when the running binary came from a local
//     clone (detected by walking up from os.Executable until we find go.mod
//     with the expected module path).
//  2. Fall back to `go install <modulePath>@latest` which writes to $GOBIN.
//  3. If neither works, explain why and return an error.
//
// Returns a human-readable status line (for printing) and any error.
func SelfUpdate() (string, error) {
	// 1. try in-place rebuild from the source repo
	if repoDir, ok := findSourceRepo(); ok {
		return rebuildInPlace(repoDir)
	}

	// 2. fall back to go install
	return goInstall()
}

// findSourceRepo walks up from the binary's real path looking for a Go module
// rooted at modulePath. Returns the directory if found.
func findSourceRepo() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		real = exe
	}
	dir := filepath.Dir(real)
	for i := 0; i < 8; i++ { // walk up at most 8 levels
		goMod := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goMod); err == nil {
			if strings.Contains(string(data), "module "+modulePath) {
				// also needs a .git dir to be updatable
				if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
					return dir, true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// rebuildInPlace runs git pull + go build inside a source repo, then copies
// the new binary on top of the currently-running one. Go on Unix allows
// replacing a running executable via rename, so this is atomic.
func rebuildInPlace(repoDir string) (string, error) {
	// 1. git pull
	pull := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
	out, err := pull.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(out)))
	}

	// 2. go build into a temp path inside the repo
	tmpBin := filepath.Join(repoDir, "remarkable.new")
	build := exec.Command("go", "build", "-o", tmpBin, ".")
	build.Dir = repoDir
	if out, err := build.CombinedOutput(); err != nil {
		os.Remove(tmpBin)
		return "", fmt.Errorf("go build failed: %s", strings.TrimSpace(string(out)))
	}
	defer os.Remove(tmpBin)

	// 3. swap into place: first the in-repo binary (the one most users symlink to)
	inRepoBin := filepath.Join(repoDir, "remarkable")
	if err := os.Rename(tmpBin, inRepoBin); err != nil {
		return "", fmt.Errorf("cannot replace repo binary: %v", err)
	}

	// 4. if the running binary is a separate file (not a symlink to inRepoBin),
	//    also copy over it so an /usr/local/bin install stays fresh
	exe, _ := os.Executable()
	real, _ := filepath.EvalSymlinks(exe)
	if real != "" && real != inRepoBin {
		if err := copyFile(inRepoBin, real); err != nil {
			return "", fmt.Errorf("rebuilt in repo but cannot update %s: %v", real, err)
		}
	}

	return fmt.Sprintf("rebuilt from source at %s", repoDir), nil
}

// goInstall runs `go install <modulePath>@latest`, which updates the binary
// in the user's $GOBIN (typically ~/go/bin/remarkable).
func goInstall() (string, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("no source repo detected and `go` is not on PATH — cannot self-update")
	}
	cmd := exec.Command("go", "install", modulePath+"@latest")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go install failed: %s", strings.TrimSpace(string(out)))
	}
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		if gopath := os.Getenv("GOPATH"); gopath != "" {
			gobin = filepath.Join(gopath, "bin")
		} else if home, err := os.UserHomeDir(); err == nil {
			gobin = filepath.Join(home, "go", "bin")
		}
	}
	return fmt.Sprintf("installed to %s/remarkable", gobin), nil
}

// copyFile copies src to dst, preserving mode. Used when the running binary
// lives in a different location than the repo build output.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// write to a sibling temp file then rename — avoids corrupting dst on failure
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// ForceCheck runs the GitHub check synchronously. Tries releases first,
// falls back to main-branch commit. Returns (tag, sha, date); tag is empty
// when no release is published yet.
func ForceCheck(ctx context.Context) (tag, sha, date string, err error) {
	tag, sha, date, err = fetchLatest(ctx)
	if err != nil {
		return "", "", "", err
	}
	c := loadCache()
	c.LastCheckUnix = time.Now().Unix()
	c.LatestTag = tag
	c.LatestSHA = sha
	c.LatestDate = date
	saveCache(c)
	return tag, sha, date, nil
}

// CurrentSHA is exported for display in the update command
func CurrentSHA() string { return currentSHA() }

// IsRemoteNewer is exported so `update --check` can distinguish "behind"
// from "ahead" without printing a misleading banner.
func IsRemoteNewer(remoteDate string) bool { return isRemoteNewer(remoteDate) }

// isRemoteNewer returns true when remoteDate parses and is strictly after
// our embedded build time.
func isRemoteNewer(remoteDate string) bool {
	if remoteDate == "" {
		return false
	}
	remote, err := time.Parse(time.RFC3339, remoteDate)
	if err != nil {
		return false
	}
	mine := currentBuildTime()
	if mine.IsZero() {
		return true // dev build: assume anything is newer
	}
	return remote.After(mine)
}
