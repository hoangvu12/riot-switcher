package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"riot-switcher/internal/account"
	"riot-switcher/internal/riot"
	"riot-switcher/internal/tui"
	"riot-switcher/internal/update"
)

func main() {
	store, err := account.OpenDefaultStore()
	if err != nil {
		fatal(err)
	}

	if len(os.Args) < 2 {
		if err := tui.Run(store); err != nil {
			fatal(err)
		}
		return
	}

	switch os.Args[1] {
	case "list", "ls":
		listProfiles(store)
	case "add", "login":
		if len(os.Args) < 3 {
			fatal(errors.New("usage: rsw add <profile-id> [label]"))
		}
		label := labelFromArgs(os.Args[2], os.Args[3:])
		if err := beginSetup(os.Args[2], label); err != nil {
			fatal(err)
		}
	case "capture", "save":
		if len(os.Args) < 3 {
			fatal(errors.New("usage: rsw capture <profile-id> [label]"))
		}
		label := labelFromArgs(os.Args[2], os.Args[3:])
		if err := captureProfile(store, os.Args[2], label); err != nil {
			fatal(err)
		}
	case "remove", "rm", "delete", "del":
		if len(os.Args) < 3 {
			fatal(errors.New("usage: rsw remove <profile-id>"))
		}
		if err := store.Remove(os.Args[2]); err != nil {
			fatal(err)
		}
		fmt.Println("removed", os.Args[2])
	case "switch", "use":
		if len(os.Args) < 3 {
			fatal(errors.New("usage: rsw switch <profile-id>"))
		}
		if err := switchProfile(store, os.Args[2]); err != nil {
			fatal(err)
		}
	case "path":
		path, err := riot.ResolveClientPath("")
		if err != nil {
			fatal(err)
		}
		fmt.Println(path)
	case "update":
		if err := updateCLI(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "help", "-h", "--help":
		printHelp()
	case "tui":
		if err := tui.Run(store); err != nil {
			fatal(err)
		}
	default:
		printHelp()
		os.Exit(2)
	}
}

func labelFromArgs(id string, args []string) string {
	if len(args) == 0 {
		return id
	}
	return strings.Join(args, " ")
}

func listProfiles(store *account.Store) {
	profiles, err := store.List()
	if err != nil {
		fatal(err)
	}
	if len(profiles) == 0 {
		fmt.Println("No profiles yet.")
		fmt.Println("Start with: rsw add main \"Main Account\"")
		return
	}
	fmt.Println("ID\tLABEL\tCAPTURED")
	for _, profile := range profiles {
		fmt.Printf("%s\t%s\t%s\n", profile.ID, profile.Label, profile.CapturedAt.Format("2006-01-02 15:04"))
	}
}

func beginSetup(id, label string) error {
	fmt.Println("Riot Client will open with a clean session.")
	fmt.Println("Log in manually, complete verification/2FA, and enable Stay signed in.")
	if label == id {
		fmt.Printf("When login finishes, run: rsw capture %s\n", id)
	} else {
		fmt.Printf("When login finishes, run: rsw capture %s %q\n", id, label)
	}
	return riot.BeginSetup(riot.Options{Log: logf})
}

func captureProfile(store *account.Store, id, label string) error {
	if err := riot.Capture(store.SnapshotDir(id), riot.Options{Log: logf}); err != nil {
		return err
	}
	if err := store.Upsert(account.Profile{ID: id, Label: label, CapturedAt: time.Now()}); err != nil {
		return err
	}
	if err := store.SetCurrent(id); err != nil {
		return err
	}
	fmt.Printf("Captured %s (%s).\n", id, label)
	fmt.Printf("Switch to it later with: rsw use %s\n", id)
	return nil
}

func switchProfile(store *account.Store, id string) error {
	current, err := store.Current()
	if err != nil {
		return err
	}
	if current != "" && current != id {
		if profile, err := store.Get(current); err == nil && riot.LiveSessionReady(riot.Options{Log: logf}) {
			fmt.Println("Saving current live session for", current)
			if err := riot.Capture(store.SnapshotDir(current), riot.Options{Log: logf}); err != nil {
				return err
			}
			profile.CapturedAt = time.Now()
			if err := store.Upsert(profile); err != nil {
				return err
			}
		}
	}
	if _, err := store.Get(id); err != nil {
		return err
	}
	if err := riot.Switch(store.SnapshotDir(id), riot.Options{Log: logf}); err != nil {
		return err
	}
	if err := store.SetCurrent(id); err != nil {
		return err
	}
	fmt.Println("Switched to", id)
	return nil
}

func logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func updateCLI(args []string) error {
	opts := update.Options{Log: logf}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			if i+1 >= len(args) {
				return errors.New("usage: rsw update [--repo owner/name] [--version tag]")
			}
			opts.Repo = args[i+1]
			i++
		case "--version":
			if i+1 >= len(args) {
				return errors.New("usage: rsw update [--repo owner/name] [--version tag]")
			}
			opts.Version = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown update option: %s", args[i])
		}
	}
	return update.Run(opts)
}

func printHelp() {
	fmt.Println(`rsw

Simple flow:
  rsw add main "Main Account"       Open Riot for manual login
  rsw save main "Main Account"      Capture that signed-in session
  rsw use main                      Switch to that account later

Commands:
  rsw                               Open the interactive TUI
  rsw list                          List profiles (alias: ls)
  rsw add <id> [label]              Start adding a profile (alias: login)
  rsw save <id> [label]             Capture current Riot session (alias: capture)
  rsw use <id>                      Restore profile and launch Riot (alias: switch)
  rsw delete <id>                   Delete profile (aliases: remove, rm, del)
  rsw path                          Print detected Riot client path
  rsw update                        Update from latest GitHub release
  rsw tui                           Open the interactive TUI`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
