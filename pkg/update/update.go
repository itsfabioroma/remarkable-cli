// Package update provides self-update and new-version awareness for the
// remarkable CLI. It checks GitHub for the latest release (or main commit as
// fallback), caches the result for a day, notifies the user non-intrusively
// when they're behind, and replaces the active binary in place on demand.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// GitHub repo the CLI lives in. Matches the module path in go.mod.
const (
	repoOwner     = "itsfabioroma"
	repoName      = "remarkable-cli"
	modulePath    = "github.com/itsfabioroma/remarkable-cli"
	checkInterval = 24 * time.Hour
)

// Cache persisted between runs.
type Cache struct {
	LastCheckUnix int64  `json:"last_check_unix"`
	LatestSHA     string `json:"latest_sha"`
	LatestDate    string `json:"latest_date"`
	LatestTag     string `json:"latest_tag,omitempty"`
	ShownForSHA   string `json:"shown_for_sha,omitempty"`
}

// cachePath is the on-disk location of the update cache.
func cachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "remarkable-cli", "update-check.json")
}

func loadCache() Cache {
	var c Cache
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

func saveCache(c Cache) {
	data, _ := json.Marshal(c)
	_ = os.MkdirAll(filepath.Dir(cachePath()), 0700)
	_ = os.WriteFile(cachePath(), data, 0600)
}

// currentSHA returns the vcs.revision embedded at build time.
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

// currentBuildTime returns the vcs.time embedded at build time.
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

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// latestRelease fetches the latest GitHub Release (tag, sha, date).
// Returns empty strings (not an error) when no release exists.
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
		return "", "", "", nil
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

// latestCommit is the fallback when no release exists yet.
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
func fetchLatest(ctx context.Context) (tag, sha, date string, err error) {
	tag, sha, date, err = latestRelease(ctx)
	if err != nil {
		return "", "", "", err
	}
	if tag != "" {
		return tag, sha, date, nil
	}
	sha, date, err = latestCommit(ctx)
	return "", sha, date, err
}

var bgOnce sync.Once

// BackgroundCheck refreshes the cache if older than checkInterval. Non-blocking.
func BackgroundCheck() {
	bgOnce.Do(func() {
		go func() {
			defer func() { _ = recover() }()
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

// Notify prints an update banner to stderr when remote is newer.
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
	if !isRemoteNewer(c.LatestDate) {
		return
	}

	fmt.Fprintf(os.Stderr, "\x1b[33m▲\x1b[0m update available: %s → %s  (run `remarkable update`)\n",
		shortSHA(cur), shortSHA(c.LatestSHA))

	c.ShownForSHA = c.LatestSHA
	saveCache(c)
}

// --- self-update ---

// releaseBaseURL is overridden in tests to point at httptest.Server.
var releaseBaseURL = fmt.Sprintf("https://github.com/%s/%s/releases/download", repoOwner, repoName)

// activeExePath returns the resolved path of the currently running binary.
// All update strategies target this path so the binary on PATH actually changes.
func activeExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return real, nil
}

// archiveName is the goreleaser-produced archive for the current OS/arch.
func archiveName() string {
	return fmt.Sprintf("remarkable_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

// SelfUpdate replaces the active binary with the latest version.
// Strategy, in order:
//  1. Download the GitHub Release tarball for this OS/arch, verify sha256,
//     atomically replace the active binary. Preferred — no toolchain needed
//     and the binary has proper ldflag version info baked in.
//  2. If running from inside a source checkout, `git pull && go build`, then
//     copy over the active binary.
//  3. `go install module@latest`, then copy the result over the active binary.
//
// Returns a status line naming the exact path that was updated.
func SelfUpdate(ctx context.Context) (string, error) {
	exe, err := activeExePath()
	if err != nil {
		return "", fmt.Errorf("cannot locate running binary: %v", err)
	}

	// 1. release tarball (preferred)
	if tag, _, _, err := latestRelease(ctx); err == nil && tag != "" {
		if msg, err := downloadAndReplace(ctx, tag, exe); err == nil {
			return msg, nil
		} else {
			// fall through to source strategies, but remember why
			fmt.Fprintf(os.Stderr, "release download failed (%v) — trying source build\n", err)
		}
	}

	// 2. in-place rebuild from source clone
	if repoDir, ok := findSourceRepo(); ok {
		return rebuildInPlace(repoDir, exe)
	}

	// 3. go install + copy over active binary
	return goInstallAndCopy(exe)
}

// downloadAndReplace fetches the release tarball + checksums for the current
// OS/arch, verifies sha256, extracts the `remarkable` binary, and atomically
// replaces `target`.
func downloadAndReplace(ctx context.Context, tag, target string) (string, error) {
	name := archiveName()
	base := fmt.Sprintf("%s/%s", releaseBaseURL, tag)

	client := &http.Client{Timeout: 60 * time.Second}

	// 1. download checksums.txt
	sums, err := httpGet(ctx, client, base+"/checksums.txt")
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %v", err)
	}
	want, ok := parseChecksum(string(sums), name)
	if !ok {
		return "", fmt.Errorf("no checksum entry for %s", name)
	}

	// 2. download the archive
	archive, err := httpGet(ctx, client, base+"/"+name)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %v", name, err)
	}

	// 3. verify sha256
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("checksum mismatch for %s", name)
	}

	// 4. extract the `remarkable` entry into a temp file next to the target
	tmp := target + ".new"
	if err := extractBinary(archive, "remarkable", tmp); err != nil {
		return "", fmt.Errorf("extract: %v", err)
	}

	// 5. atomic replace
	if err := os.Chmod(tmp, 0755); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("replace %s: %v", target, err)
	}

	return fmt.Sprintf("updated %s to %s (%s/%s release)", target, tag, runtime.GOOS, runtime.GOARCH), nil
}

// httpGet is a small helper that reads a GET response into memory with a ctx.
func httpGet(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "remarkable-cli-update")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseChecksum finds the sha256 for `name` in goreleaser's `<hash>  <file>`
// checksums.txt format. Tolerates a leading `*` on the filename (binary mode).
func parseChecksum(body, name string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		f := strings.TrimPrefix(fields[1], "*")
		if f == name {
			return fields[0], true
		}
	}
	return "", false
}

// extractBinary walks a tar.gz in memory looking for `want` and writes it to dst.
func extractBinary(archive []byte, want, dst string) error {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("binary %q not found in archive", want)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != want {
			continue
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			os.Remove(dst)
			return err
		}
		return out.Close()
	}
}

// findSourceRepo walks up from the running binary looking for a Go module
// rooted at modulePath with a .git directory.
func findSourceRepo() (string, bool) {
	exe, err := activeExePath()
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(exe)
	for i := 0; i < 8; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module "+modulePath) {
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

// rebuildInPlace runs git pull + go build in the repo, then copies the binary
// over the currently-running executable path.
func rebuildInPlace(repoDir, exe string) (string, error) {
	pull := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
	if out, err := pull.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git pull failed: %s", strings.TrimSpace(string(out)))
	}

	tmpBin := filepath.Join(repoDir, "remarkable.new")
	build := exec.Command("go", "build", "-o", tmpBin, ".")
	build.Dir = repoDir
	if out, err := build.CombinedOutput(); err != nil {
		os.Remove(tmpBin)
		return "", fmt.Errorf("go build failed: %s", strings.TrimSpace(string(out)))
	}
	defer os.Remove(tmpBin)

	if err := copyFile(tmpBin, exe); err != nil {
		return "", fmt.Errorf("cannot update %s: %v", exe, err)
	}
	return fmt.Sprintf("rebuilt from %s and updated %s", repoDir, exe), nil
}

// goInstallAndCopy runs `go install` then copies the result over the active
// binary so the thing on PATH actually changes.
func goInstallAndCopy(exe string) (string, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("no release match, no source repo, and `go` is not on PATH — cannot self-update")
	}
	cmd := exec.Command("go", "install", modulePath+"@latest")
	if out, err := cmd.CombinedOutput(); err != nil {
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
	installed := filepath.Join(gobin, "remarkable")

	// sanity check: the thing we just installed has to exist
	if _, err := os.Stat(installed); err != nil {
		return "", fmt.Errorf("go install succeeded but %s missing: %v", installed, err)
	}

	// copy onto the active binary if it's a different file
	if installed != exe {
		if err := copyFile(installed, exe); err != nil {
			return "", fmt.Errorf("installed to %s but cannot update %s: %v", installed, exe, err)
		}
		return fmt.Sprintf("go install → %s and updated %s", installed, exe), nil
	}
	return fmt.Sprintf("go install → %s", installed), nil
}

// copyFile atomically replaces dst with the contents of src, preserving 0755.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

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

// ForceCheck runs the GitHub check synchronously.
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

// CurrentSHA is exported for the `update` command.
func CurrentSHA() string { return currentSHA() }

// ActiveExePath is exported so the `update` command can report the path.
func ActiveExePath() (string, error) { return activeExePath() }

// IsRemoteNewer is exported so `update --check` can distinguish "behind" from "ahead".
func IsRemoteNewer(remoteDate string) bool { return isRemoteNewer(remoteDate) }

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
		return true
	}
	return remote.After(mine)
}
