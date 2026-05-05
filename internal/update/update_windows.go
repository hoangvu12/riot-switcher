package update

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultRepo = "hoangvu12/riot-switcher"
const assetName = "rsw-windows-amd64.exe"
const checksumName = "checksums.txt"

type Options struct {
	Repo    string
	Version string
	Log     func(format string, args ...any)
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func Run(opts Options) error {
	log := opts.Log
	if log == nil {
		log = func(string, ...any) {}
	}
	repo := opts.Repo
	if repo == "" {
		repo = os.Getenv("RIOT_SWITCHER_REPO")
	}
	if repo == "" {
		repo = defaultRepo
	}
	if !strings.Contains(repo, "/") {
		return errors.New("repo must be in owner/name form")
	}

	version := opts.Version
	if version == "" {
		version = "latest"
	}

	rel, err := fetchRelease(repo, version)
	if err != nil {
		return err
	}
	assetURL := ""
	checksumURL := ""
	for _, asset := range rel.Assets {
		if asset.Name == assetName {
			assetURL = asset.URL
		}
		if asset.Name == checksumName {
			checksumURL = asset.URL
		}
	}
	if assetURL == "" {
		return fmt.Errorf("release %s does not contain %s", rel.TagName, assetName)
	}
	if checksumURL == "" {
		return fmt.Errorf("release %s does not contain %s", rel.TagName, checksumName)
	}

	log("fetching checksums")
	checksums, err := downloadBytes(checksumURL)
	if err != nil {
		return err
	}
	expectedHash, err := checksumFor(checksums, assetName)
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(os.TempDir(), assetName)
	log("downloading %s", assetURL)
	if err := download(assetURL, tmpPath, log); err != nil {
		return err
	}
	if err := verifyChecksum(tmpPath, expectedHash); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	log("installing %s", rel.TagName)
	return replaceAfterExit(exePath, tmpPath)
}

func fetchRelease(repo, version string) (release, error) {
	endpoint := "https://api.github.com/repos/" + repo + "/releases/latest"
	if version != "latest" {
		endpoint = "https://api.github.com/repos/" + repo + "/releases/tags/" + version
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("User-Agent", "rsw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return release{}, fmt.Errorf("GitHub release lookup failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return release{}, err
	}
	return rel, nil
}

func downloadBytes(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rsw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func download(url, target string, log func(format string, args ...any)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "rsw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, &progressReader{reader: resp.Body, total: resp.ContentLength, log: log})
	if err == nil {
		log("download complete")
	}
	return err
}

type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	last   time.Time
	log    func(format string, args ...any)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		if time.Since(r.last) > time.Second {
			r.last = time.Now()
			if r.total > 0 {
				r.log("downloaded %.1f/%.1f MB", bytesToMB(r.read), bytesToMB(r.total))
			} else {
				r.log("downloaded %.1f MB", bytesToMB(r.read))
			}
		}
	}
	return n, err
}

func bytesToMB(n int64) float64 {
	return float64(n) / 1024 / 1024
}

func checksumFor(data []byte, name string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("checksum entry not found for %s", name)
}

func verifyChecksum(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", hash.Sum(nil))
	if actual != strings.ToLower(expected) {
		return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
	}
	return nil
}

func replaceAfterExit(exePath, tmpPath string) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$pidToWait = %d
$source = '%s'
$target = '%s'
Wait-Process -Id $pidToWait -ErrorAction SilentlyContinue
Move-Item -LiteralPath $source -Destination $target -Force
Write-Host "Updated: $target"
`, os.Getpid(), psQuote(tmpPath), psQuote(exePath))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Println("update downloaded; installer will replace the executable after this process exits")
	return nil
}

func psQuote(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
