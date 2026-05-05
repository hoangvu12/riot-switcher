package riot

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var clientProcesses = []string{
	"RiotClientServices.exe",
	"RiotClientUx.exe",
	"RiotClientUxRender.exe",
	"Riot Client.exe",
}

var gameProcesses = []string{
	"LeagueofLegends.exe",
	"VALORANT-Win64-Shipping.exe",
}

type Options struct {
	RiotClientPath string
	Log            func(format string, args ...any)
}

type snapshotItem struct {
	Name     string
	Path     func(installDir string) (string, error)
	Required bool
	Dir      bool
	Ignore   map[string]bool
}

func BeginSetup(opts Options) error {
	log := logger(opts.Log)
	path, err := ResolveClientPath(opts.RiotClientPath)
	if err != nil {
		return err
	}
	if err := ensureNoGameRunning(); err != nil {
		return err
	}
	log("closing Riot Client for clean login")
	gracefulQuit()
	_ = killProcesses(clientProcesses...)
	if err := waitProcessesGone(10*time.Second, clientProcesses...); err != nil {
		return err
	}
	log("clearing live Riot session files")
	if err := clearLiveState(filepath.Dir(path)); err != nil {
		return err
	}
	log("launching Riot Client; log in manually and enable Stay signed in")
	return launchRiot(path)
}

func Capture(snapshotDir string, opts Options) error {
	log := logger(opts.Log)
	path, err := ResolveClientPath(opts.RiotClientPath)
	if err != nil {
		return err
	}
	installDir := filepath.Dir(path)
	ready, err := settingsReady(installDir)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("Riot session is not ready; log in with Stay signed in enabled, then try capture again")
	}
	log("closing Riot Client so persisted session files flush to disk")
	gracefulQuit()
	_ = killProcesses(clientProcesses...)
	if err := waitProcessesGone(10*time.Second, clientProcesses...); err != nil {
		return err
	}
	log("capturing Riot session snapshot")
	return backupLiveSnapshot(snapshotDir, installDir)
}

func Switch(snapshotDir string, opts Options) error {
	log := logger(opts.Log)
	path, err := ResolveClientPath(opts.RiotClientPath)
	if err != nil {
		return err
	}
	if err := ensureNoGameRunning(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(snapshotDir, "RiotGamesPrivateSettings.yaml")); err != nil {
		return fmt.Errorf("profile snapshot is missing RiotGamesPrivateSettings.yaml: %w", err)
	}
	log("closing Riot Client")
	gracefulQuit()
	_ = killProcesses(clientProcesses...)
	if err := waitProcessesGone(10*time.Second, clientProcesses...); err != nil {
		return err
	}
	log("restoring Riot session snapshot")
	if err := restoreLiveSnapshot(snapshotDir, filepath.Dir(path)); err != nil {
		return err
	}
	log("launching Riot Client")
	return launchRiot(path)
}

func ResolveClientPath(override string) (string, error) {
	candidates := []string{
		override,
		os.Getenv("RIOT_CLIENT_PATH"),
		detectInstallationPath(),
		`C:\Riot Games\Riot Client\RiotClientServices.exe`,
		filepath.Join(os.Getenv("ProgramFiles"), `Riot Games\Riot Client\RiotClientServices.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Riot Games\Riot Client\RiotClientServices.exe`),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("RiotClientServices.exe not found; set RIOT_CLIENT_PATH")
}

func snapshotItems() []snapshotItem {
	return []snapshotItem{
		{Name: "RiotGamesPrivateSettings.yaml", Required: true, Path: localPath(`Riot Games\Riot Client\Data\RiotGamesPrivateSettings.yaml`)},
		{Name: "LeagueRiotGamesPrivateSettings.yaml", Path: localPath(`Riot Games\League of Legends\Data\RiotGamesPrivateSettings.yaml`)},
		{Name: "Sessions", Dir: true, Path: localPath(`Riot Games\Riot Client\Data\Sessions`)},
		{Name: "RiotClientConfig", Dir: true, Path: localPath(`Riot Games\Riot Client\Config`), Ignore: map[string]bool{"lockfile": true}},
		{Name: "RiotMetadata", Dir: true, Path: programDataPath(`Riot Games\Metadata\Riot Client`)},
		{Name: "InstallConfig", Dir: true, Path: func(installDir string) (string, error) { return filepath.Join(installDir, "Config"), nil }},
	}
}

func backupLiveSnapshot(snapshotDir, installDir string) error {
	if err := os.RemoveAll(snapshotDir); err != nil {
		return err
	}
	if err := os.MkdirAll(snapshotDir, 0700); err != nil {
		return err
	}
	captured := false
	for _, item := range snapshotItems() {
		source, err := item.Path(installDir)
		if err != nil {
			if item.Required {
				return err
			}
			continue
		}
		target := filepath.Join(snapshotDir, item.Name)
		if _, err := os.Stat(source); err != nil {
			if item.Required {
				return fmt.Errorf("required Riot session file not found: %s", source)
			}
			continue
		}
		if err := copyPath(source, target, item.Ignore); err != nil {
			return err
		}
		captured = true
	}
	if !captured {
		return errors.New("no Riot session files were captured")
	}
	return nil
}

func restoreLiveSnapshot(snapshotDir, installDir string) error {
	if err := clearLiveState(installDir); err != nil {
		return err
	}
	for _, item := range snapshotItems() {
		source := filepath.Join(snapshotDir, item.Name)
		if _, err := os.Stat(source); err != nil {
			if item.Required {
				return fmt.Errorf("required profile snapshot file not found: %s", source)
			}
			continue
		}
		target, err := item.Path(installDir)
		if err != nil {
			if item.Required {
				return err
			}
			continue
		}
		if err := copyPath(source, target, item.Ignore); err != nil {
			return err
		}
	}
	return nil
}

func clearLiveState(installDir string) error {
	for _, item := range snapshotItems() {
		path, err := item.Path(installDir)
		if err != nil {
			if item.Required {
				return err
			}
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func settingsReady(installDir string) (bool, error) {
	path, err := localPath(`Riot Games\Riot Client\Data\RiotGamesPrivateSettings.yaml`)(installDir)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() <= 1000 {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.Contains(data, []byte("offline_access")), nil
}

func localPath(relative string) func(string) (string, error) {
	return func(string) (string, error) {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", errors.New("LOCALAPPDATA is not set")
		}
		return filepath.Join(base, relative), nil
	}
}

func programDataPath(relative string) func(string) (string, error) {
	return func(string) (string, error) {
		base := os.Getenv("PROGRAMDATA")
		if base == "" {
			return "", errors.New("PROGRAMDATA is not set")
		}
		return filepath.Join(base, relative), nil
	}
}

func copyPath(source, target string, ignored map[string]bool) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(source, target)
	}
	return copyDir(source, target, ignored)
}

func copyDir(source, target string, ignored map[string]bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	for _, entry := range entries {
		if ignored != nil && ignored[strings.ToLower(entry.Name())] {
			continue
		}
		if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()), ignored); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func ensureNoGameRunning() error {
	running, err := runningProcesses()
	if err != nil {
		return err
	}
	var active []string
	for _, name := range gameProcesses {
		if running[strings.ToLower(name)] {
			active = append(active, name)
		}
	}
	if len(active) > 0 {
		return fmt.Errorf("close Riot game processes before switching: %s", strings.Join(active, ", "))
	}
	return nil
}

func gracefulQuit() {
	lockfile, err := riotLockfile()
	if err != nil {
		return
	}
	data, err := os.ReadFile(lockfile)
	if err != nil {
		return
	}
	parts := strings.Split(strings.TrimSpace(string(data)), ":")
	if len(parts) < 5 {
		return
	}
	url := fmt.Sprintf("%s://127.0.0.1:%s/process-control/v1/process/quit", parts[4], parts[2])
	password := powershellSingleQuoted(parts[3])
	script := fmt.Sprintf(`
[System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
$pair = 'riot:%s'
$bytes = [System.Text.Encoding]::ASCII.GetBytes($pair)
$auth = [Convert]::ToBase64String($bytes)
try { Invoke-WebRequest -UseBasicParsing -Method Post -Uri '%s' -Headers @{ Authorization = "Basic $auth" } | Out-Null } catch { }
`, password, powershellSingleQuoted(url))
	_ = exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Run()
	time.Sleep(1500 * time.Millisecond)
}

func riotLockfile() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(base, `Riot Games\Riot Client\Config\lockfile`), nil
}

func killProcesses(names ...string) error {
	for _, name := range names {
		_ = exec.Command("taskkill", "/IM", name, "/T", "/F").Run()
	}
	return nil
}

func launchRiot(path string) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		fmt.Sprintf(`Start-Process -FilePath %q -WorkingDirectory %q -ArgumentList '--launch-product=riot-client','--launch-patchline=live'`, path, filepath.Dir(path)))
	return cmd.Run()
}

func waitProcessesGone(timeout time.Duration, names ...string) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := runningProcesses()
		if err != nil {
			return err
		}
		if !anyRunning(running, names...) {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for processes to stop: %s", strings.Join(names, ", "))
}

func runningProcesses() (map[string]bool, error) {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(bytes.NewReader(out))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	processes := make(map[string]bool, len(rows))
	for _, row := range rows {
		if len(row) > 0 {
			processes[strings.ToLower(row[0])] = true
		}
	}
	return processes, nil
}

func anyRunning(running map[string]bool, names ...string) bool {
	for _, name := range names {
		if running[strings.ToLower(name)] {
			return true
		}
	}
	return false
}

func detectInstallationPath() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(programData, `Riot Games\RiotClientInstalls.json`))
	if err != nil {
		return ""
	}
	for _, key := range []string{"rc_live", "rc_default"} {
		needle := fmt.Sprintf("\"%s\"", key)
		idx := bytes.Index(data, []byte(needle))
		if idx < 0 {
			continue
		}
		rest := data[idx+len(needle):]
		colon := bytes.IndexByte(rest, ':')
		if colon < 0 {
			continue
		}
		rest = bytes.TrimSpace(rest[colon+1:])
		if len(rest) == 0 || rest[0] != '"' {
			continue
		}
		rest = rest[1:]
		end := bytes.IndexByte(rest, '"')
		if end < 0 {
			continue
		}
		candidate := strings.ReplaceAll(string(rest[:end]), `\\`, `\`)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func logger(log func(format string, args ...any)) func(format string, args ...any) {
	if log != nil {
		return log
	}
	return func(string, ...any) {}
}

func powershellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
