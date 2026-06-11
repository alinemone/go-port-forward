// Package updater handles self-update of the pf binary from GitHub releases.
package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	repoOwner = "alinemone"
	repoName  = "go-port-forward"

	apiLatestURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"

	httpTimeout     = 15 * time.Second
	downloadTimeout = 5 * time.Minute
)

// Options controls update behavior.
type Options struct {
	CurrentVersion string // version.Version of the running binary
	AssumeYes      bool   // skip confirmation prompt
	Force          bool   // re-install even if already on latest
}

// asset is one file in a GitHub release.
type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []asset `json:"assets"`
}

// Run performs the full update flow.
func Run(opts Options) error {
	exePath, err := currentExePath()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}

	wantAsset := assetName()
	fmt.Printf("Checking for updates...\n")
	fmt.Printf("  current : %s\n", displayVersion(opts.CurrentVersion))
	fmt.Printf("  binary  : %s\n", exePath)
	fmt.Printf("  asset   : %s\n", wantAsset)
	fmt.Println()

	rel, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}

	binAsset, sumsAsset, err := pickAssets(rel, wantAsset)
	if err != nil {
		return err
	}

	cmp, err := compareVersions(opts.CurrentVersion, rel.TagName)
	if err != nil {
		cmp = -1
	}

	fmt.Printf("  latest  : %s  (%s)\n", rel.TagName, shortDate(rel.PublishedAt))
	fmt.Printf("  size    : %.2f MB\n", float64(binAsset.Size)/(1024*1024))
	fmt.Println()

	if cmp >= 0 && !opts.Force {
		fmt.Println("✓ You are already on the latest version.")
		return nil
	}

	if !opts.AssumeYes && !confirm(fmt.Sprintf("Update %s → %s?", displayVersion(opts.CurrentVersion), rel.TagName)) {
		fmt.Println("Aborted.")
		return nil
	}

	if err := checkWritable(filepath.Dir(exePath)); err != nil {
		fmt.Printf("⚠ No write permission to %s\n", filepath.Dir(exePath))
		fmt.Println("  Attempting to relaunch with elevated privileges...")
		if elevErr := tryElevate(); elevErr != nil {
			return fmt.Errorf("elevation failed: %w (original: %v)", elevErr, err)
		}
		return errors.New("elevation did not exit; please re-run with sudo or as administrator")
	}

	tmpDir, err := os.MkdirTemp("", "pf-update-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, wantAsset)
	fmt.Printf("⇣ Downloading %s ...\n", binAsset.Name)
	if err := downloadFile(binAsset.BrowserDownloadURL, binPath); err != nil {
		return fmt.Errorf("download binary: %w", err)
	}

	if sumsAsset == nil {
		return fmt.Errorf("SHA256SUMS.txt not found in release %s; refusing to install an unverified binary", rel.TagName)
	}
	fmt.Println("✓ Verifying SHA256...")
	sumsPath := filepath.Join(tmpDir, sumsAsset.Name)
	if err := downloadFile(sumsAsset.BrowserDownloadURL, sumsPath); err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(binPath, sumsPath, wantAsset); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	fmt.Printf("→ Installing to %s ...\n", exePath)
	if err := replaceBinary(binPath, exePath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("✓ Updated to %s\n", rel.TagName)
	fmt.Println("  Run 'pf version' to confirm.")
	return nil
}

func currentExePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p), nil
}

// assetName returns the release asset name for the running platform.
// Mirrors .github/workflows/release.yml output naming.
func assetName() string {
	name := fmt.Sprintf("pf-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func fetchLatestRelease() (*release, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest("GET", apiLatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pf-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}
	return &rel, nil
}

func pickAssets(rel *release, wantBinary string) (*asset, *asset, error) {
	var binAsset, sumsAsset *asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		switch a.Name {
		case wantBinary:
			binAsset = a
		case "SHA256SUMS.txt":
			sumsAsset = a
		}
	}
	if binAsset == nil {
		available := make([]string, 0, len(rel.Assets))
		for _, a := range rel.Assets {
			available = append(available, a.Name)
		}
		return nil, nil, fmt.Errorf("no asset %q in release %s (available: %s)",
			wantBinary, rel.TagName, strings.Join(available, ", "))
	}
	return binAsset, sumsAsset, nil
}

func downloadFile(url, dst string) error {
	client := &http.Client{Timeout: downloadTimeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pf-updater")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

func verifyChecksum(binPath, sumsPath, assetName string) error {
	want, err := lookupHash(sumsPath, assetName)
	if err != nil {
		return err
	}

	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", got, want)
	}
	return nil
}

// lookupHash parses a `sha256sum`-style file and returns the hash for the given filename.
// Lines look like: "<hex>  filename" (two spaces between).
func lookupHash(sumsPath, name string) (string, error) {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		fname := fields[len(fields)-1]
		if fname == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum line for %s in SHA256SUMS.txt", name)
}

// checkWritable returns nil if the process can write into dir.
func checkWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".pf-update-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return nil
}

func confirm(prompt string) bool {
	fmt.Printf("%s [Y/n]: ", prompt)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "dev (unversioned build)"
	}
	return v
}

// elevatedArgs builds the argv used when re-launching ourselves with elevated
// privileges. It preserves the original "update" subcommand and any flags
// the user passed, then forces --yes so the elevated copy skips the confirm
// prompt the user already answered.
func elevatedArgs() []string {
	out := make([]string, 0, len(os.Args))
	hasYes := false
	for _, a := range os.Args[1:] {
		out = append(out, a)
		if a == "--yes" || a == "-y" {
			hasYes = true
		}
	}
	if !hasYes {
		out = append(out, "--yes")
	}
	return out
}

func shortDate(iso string) string {
	if len(iso) >= 10 {
		return iso[:10]
	}
	return iso
}

// compareVersions returns -1 if current<latest, 0 if equal, 1 if current>latest.
// Both inputs may carry a leading "v". Non-semver inputs return an error.
func compareVersions(current, latest string) (int, error) {
	c, err := parseSemver(current)
	if err != nil {
		return 0, err
	}
	l, err := parseSemver(latest)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if c[i] != l[i] {
			if c[i] < l[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func parseSemver(v string) ([3]int, error) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return out, fmt.Errorf("invalid semver: %q", v)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("invalid semver component %q: %w", p, err)
		}
		out[i] = n
	}
	return out, nil
}
