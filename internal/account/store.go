package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Profile struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	CapturedAt time.Time `json:"capturedAt"`
}

type Store struct {
	dir  string
	path string
}

func OpenDefaultStore() (*Store, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil, errors.New("LOCALAPPDATA is not set")
	}
	dir := filepath.Join(base, "riot-switcher")
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, path: filepath.Join(dir, "profiles.json")}, nil
}

func (s *Store) SnapshotDir(id string) string {
	return filepath.Join(s.dir, "profiles", id)
}

func (s *Store) CurrentPath() string {
	return filepath.Join(s.dir, "current")
}

func (s *Store) Current() (string, error) {
	data, err := os.ReadFile(s.CurrentPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
}

func (s *Store) SetCurrent(id string) error {
	return os.WriteFile(s.CurrentPath(), []byte(id), 0600)
}

func (s *Store) List() ([]Profile, error) {
	profiles, err := s.load()
	if err != nil {
		return nil, err
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

func (s *Store) Get(id string) (Profile, error) {
	profiles, err := s.load()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("profile %q not found", id)
}

func (s *Store) Upsert(profile Profile) error {
	if profile.ID == "" {
		return errors.New("profile id is required")
	}
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	if profile.CapturedAt.IsZero() {
		profile.CapturedAt = time.Now()
	}

	profiles, err := s.load()
	if err != nil {
		return err
	}
	for i, existing := range profiles {
		if existing.ID == profile.ID {
			profiles[i] = profile
			return s.save(profiles)
		}
	}
	profiles = append(profiles, profile)
	return s.save(profiles)
}

func (s *Store) Remove(id string) error {
	profiles, err := s.load()
	if err != nil {
		return err
	}
	next := profiles[:0]
	found := false
	for _, profile := range profiles {
		if profile.ID == id {
			found = true
			continue
		}
		next = append(next, profile)
	}
	if !found {
		return fmt.Errorf("profile %q not found", id)
	}
	if err := os.RemoveAll(s.SnapshotDir(id)); err != nil {
		return err
	}
	if current, err := s.Current(); err != nil {
		return err
	} else if current == id {
		_ = os.Remove(s.CurrentPath())
	}
	return s.save(next)
}

func (s *Store) load() ([]Profile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *Store) save(profiles []Profile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
