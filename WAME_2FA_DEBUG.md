# Wame 2FA Session Restore Debug Handoff

## Project

Repo: `hoangvu12/riot-switcher`

Local path: `C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher`

Tool: `rsw`, a Windows Go CLI/TUI that switches Riot accounts by saving/restoring Riot Client local session files. It does not store passwords.

## Problem

Most saved Riot profiles restore correctly. Profile `wame` does not restore login state. `wame` uses 2FA verification.

After saving `wame`, switching to another working profile (`ame`), then switching back to `wame`, Riot Client opens logged out.

The tool restores the saved `wame` session file successfully, but Riot Client rewrites it to a small logged-out file after launch.

## Profiles

Profile metadata file:

```text
%LOCALAPPDATA%\riot-switcher\profiles.json
```

Current profiles:

```json
[
  { "id": "CalicoCat", "label": "CalicoCat" },
  { "id": "ame", "label": "ame" },
  { "id": "wame", "label": "wame" }
]
```

Saved profile snapshots are under:

```text
%LOCALAPPDATA%\riot-switcher\profiles\<profile-id>
```

## Current Snapshot Items

The code captures/restores these Riot paths in `internal/riot/switcher_windows.go`:

```text
%LOCALAPPDATA%\Riot Games\Riot Client\Data\RiotGamesPrivateSettings.yaml
%LOCALAPPDATA%\Riot Games\League of Legends\Data\RiotGamesPrivateSettings.yaml
%LOCALAPPDATA%\Riot Games\Riot Client\Data\Sessions
%LOCALAPPDATA%\Riot Games\Riot Client\HttpCache
%LOCALAPPDATA%\Riot Games\Riot Client\Config
%PROGRAMDATA%\Riot Games\Metadata\Riot Client
<Riot install dir>\Config
```

`HttpCache` was added as a test, but did not fix `wame`.

## Recent Code Changes Under Test

Commit currently on `main` but not released:

```text
147ecd9 validate riot session capture
```

Changes made:

1. Added Riot local API check before capture:
   - reads lockfile from `%LOCALAPPDATA%\Riot Games\Riot Client\Config\lockfile`
   - calls `/riot-login/v1/status`
   - refuses capture unless `phase == logged_in` and `persist == true`
2. Gracefully quits Riot before capture to flush tokens.
3. Tracks current profile in `%LOCALAPPDATA%\riot-switcher\current`.
4. Before switching away, auto-backs up the current live profile if the live session is still valid and persistent.
5. Logs restored `RiotGamesPrivateSettings.yaml` file size.
6. Added `HttpCache` to snapshot items after first test failed.

No new release was published. A premature `v0.2.1` tag was deleted and its workflow canceled before publishing assets.

## Reproduction Steps

Run from repo root:

```powershell
cd "C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher"
```

Start clean login for `wame`:

```powershell
go run . add wame
```

In Riot Client:

```text
Log into wame
Complete 2FA
Enable Stay signed in / remember device if shown
Wait until Riot fully loads
```

Save `wame`:

```powershell
go run . save wame
```

Switch to working profile `ame`:

```powershell
go run . use ame
```

Switch back to `wame`:

```powershell
go run . use wame
```

## Observed CLI Output

```text
PS C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher> go run . add wame
Riot Client will open with a clean session.
Log in manually, complete verification/2FA, and enable Stay signed in.
When login finishes, run: rsw capture wame
closing Riot Client for clean login
clearing live Riot session files
launching Riot Client; log in manually and enable Stay signed in

PS C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher> go run . save wame
closing Riot Client so persisted session files flush to disk
capturing Riot session snapshot
Captured wame (wame).
Switch to it later with: rsw use wame

PS C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher> go run . use ame
closing Riot Client
restoring Riot session snapshot
restored Riot session file (2800 bytes)
launching Riot Client
Switched to ame

PS C:\Users\HP MEDIA\Desktop\nguyenvu\riot-switcher> go run . use wame
Saving current live session for ame
closing Riot Client so persisted session files flush to disk
capturing Riot session snapshot
closing Riot Client
restoring Riot session snapshot
restored Riot session file (2799 bytes)
launching Riot Client
Switched to wame
```

Result: Riot Client opens logged out for `wame`.

## Key Evidence

After `go run . use wame`, before/after comparison showed:

```text
Saved wame private: 2799 bytes
Live private after Riot launch: 484 bytes
```

So the tool successfully restores the full saved session file, then Riot Client starts and rewrites it to a small logged-out/default file.

With `HttpCache` capture enabled:

```text
Saved wame private: 2799 bytes
Live private: 484 bytes
Saved wame cache: 5 files, 165420 bytes
Live cache: 7 files, 317166 bytes
```

Adding `HttpCache` did not fix the issue.

## Relevant Logs

Latest Riot logs are under:

```text
%LOCALAPPDATA%\Riot Games\Riot Client\Logs\Riot Client Logs
```

Example log after failed `wame` restore:

```text
2026-05-06T04-52-38_18724_Riot Client.log
```

Repeated log lines include:

```text
SDK: rso-auth: No client authorization
SDK: rso-auth: in plugin library function HandleAuthorizationGetRequest: No authorization currently exists for client: client riot-client
SDK: player-preferences: Unauthorized: Player must be signed in
SDK: rso-auth: Logout reason not available
```

Interpretation: Riot loads after restore, but does not find/accept client authorization for `wame` and clears the session.

## GitHub Research Already Done

Searched with GitHub CLI:

```powershell
gh search repos "riot account switcher" --limit 20
gh search code "RiotGamesPrivateSettings.yaml" --limit 30
gh search code "Riot Client\\Data\\RiotGamesPrivateSettings.yaml" --limit 30
```

Useful repos found:

```text
return-zero-0/riot-session-switcher
SokvisalMong/RiotSwitcherMinimal
klNuno/accshift
arthiee4/RiotSwitcher
```

Most simple tools only swap `RiotGamesPrivateSettings.yaml` or the whole Riot `Data` folder.

`klNuno/accshift` is the most relevant mature implementation. It:

```text
checks /riot-login/v1/status
requires logged_in and persist=true
requires settings file size > 1000
gracefully quits before capture
backs up current profile before switching away
captures similar paths to this project
```

Those ideas were implemented locally, but `wame` still fails.

## Hypotheses

1. `wame` 2FA/trusted-device session is server-invalidated after restore or after another account is used.
2. Riot stores some 2FA/trusted-device material in a path not currently captured.
3. The restored auth cookies/tokens in `RiotGamesPrivateSettings.yaml` are valid at capture time but rejected on next startup for this account.
4. The issue may be account-specific, not generic, since `ame` and `CalicoCat` work.

## Things To Investigate Next

1. Diff all files modified during a manual successful `wame` login, not only known Riot folders.
2. Monitor file writes during Riot login and startup with ProcMon or PowerShell snapshots.
3. Compare working profile (`ame`) and failing profile (`wame`) startup logs around `rso-auth` and `riot-login`.
4. Check whether `wame` creates different cookies, scopes, trust levels, or account security requirements.
5. Test restoring by swapping the whole `%LOCALAPPDATA%\Riot Games\Riot Client` folder except logs/crashes, to determine whether missing local files are the cause.
6. If whole-folder restore still fails, assume Riot server invalidates the remembered session for this account.

## Current Recommendation

Do not release the current changes as a fix for `wame`; the tested result shows the account still opens logged out.

The current code changes may still be useful safety improvements, but they do not solve the 2FA `wame` restore problem.
