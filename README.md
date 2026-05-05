# rsw

Small Windows CLI for switching Riot accounts without saving Riot passwords.

`rsw` works by saving the Riot Client's remembered login session after you sign in manually. That means accounts with email verification or 2FA work normally.

## Install

One-line install from GitHub Releases:

```powershell
irm https://raw.githubusercontent.com/hoangvu12/riot-switcher/main/scripts/install-from-github.ps1 | iex
```

Open a new terminal after installing, then check it works:

```powershell
rsw
```

The GitHub installer uses `curl.exe` when available for faster release downloads, then verifies the downloaded `.exe` against the release checksum before installing.

Update later:

```powershell
rsw update
```

Uninstall:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1
```

## Quick Start

Add your first account:

```powershell
rsw add main "Main Account"
```

Riot Client will open. In Riot Client:

1. Log in manually.
2. Complete email verification or 2FA if prompted.
3. Enable `Stay signed in`.
4. Do not click Riot's sign out button.

Save that logged-in session:

```powershell
rsw save main "Main Account"
```

Add another account:

```powershell
rsw add alt "Alt Account"
```

Log in manually again, enable `Stay signed in`, then save it:

```powershell
rsw save alt "Alt Account"
```

Switch accounts:

```powershell
rsw
rsw use main
rsw use alt
```

List saved profiles:

```powershell
rsw list
```

Delete a profile:

```powershell
rsw delete alt
```

## Commands

```text
rsw                      Open the interactive TUI
rsw list                 List profiles
rsw add <id> [label]     Open Riot Client with a clean login session
rsw save <id> [label]    Save the currently signed-in Riot session
rsw use <id>             Restore a saved profile and launch Riot Client
rsw delete <id>          Delete a saved profile
rsw path                 Show detected Riot Client path
rsw update               Update rsw from GitHub Releases
rsw tui                  Open the interactive TUI explicitly
```

TUI keys:

```text
up/down, j/k  Move selection
enter         Switch to selected profile
a             Add a profile and open Riot for manual login
s             Save the current Riot session into a profile
d             Delete selected profile
r             Refresh profiles
q             Quit
```

In the TUI, adding an account is a two-step flow:

```text
1. Start Login       Opens Riot with a clean login state. Nothing is saved yet.
2. Capture Login     After you sign in to Riot, saves the current logged-in session.
```

`Capture Login` saves whichever Riot account is currently logged in on this PC. If you just used `Start Login`, it defaults to that pending profile instead of the selected row, so you do not accidentally overwrite another account.

Aliases:

```text
list:    ls
add:     login
save:    capture
use:     switch
delete:  remove, rm, del
```

## Important Notes

- Do not manually sign out in Riot Client before saving a profile. Signing out can invalidate the remembered session.
- Always run `rsw save <id>` before using `rsw add` for another account.
- Close League/Valorant before switching. `rsw` refuses to switch while game processes are running.
- Captured sessions can expire. If Riot opens on the login screen, log in again and run `rsw save <id>` again.

## Where Data Is Stored

Profiles are stored here:

```text
%LOCALAPPDATA%\riot-switcher
```

No Riot password is stored. Each profile is a local snapshot of Riot Client session files.

## Custom Riot Path

Most installs are detected automatically. If Riot is installed somewhere unusual, set:

```powershell
$env:RIOT_CLIENT_PATH = "D:\Riot Games\Riot Client\RiotClientServices.exe"
```

To make it permanent for your Windows user:

```powershell
[Environment]::SetEnvironmentVariable("RIOT_CLIENT_PATH", "D:\Riot Games\Riot Client\RiotClientServices.exe", "User")
```

## From Source

```powershell
git clone https://github.com/hoangvu12/riot-switcher.git
cd riot-switcher
powershell -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

## Release Notes

The GitHub installer expects each release to include:

```text
rsw-windows-amd64.exe
checksums.txt
```

The included GitHub Actions workflow creates this asset when a tag like `v0.1.0` is pushed.

## Disclaimer

This project is not affiliated with Riot Games. Use at your own risk and check Riot's terms before using account/session automation.
