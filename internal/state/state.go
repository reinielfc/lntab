package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// EntryKind distinguishes what was created.
type EntryKind string

const (
	KindLink EntryKind = "link" // symlink
	KindDir  EntryKind = "dir"  // directory created by lntab
)

// Entry records a single filesystem object created by lntab.
type Entry struct {
	Kind   EntryKind `json:"kind"`
	Path   string    `json:"path"`
	Group  string    `json:"group"`
	Target string    `json:"target,omitempty"` // symlink target, set for KindLink
}

// State holds all recorded entries.
type State struct {
	Entries []Entry
}

// Add appends an entry.
func (s *State) Add(e Entry) {
	s.Entries = append(s.Entries, e)
}

// Lookup returns a pointer to the entry with the given path, or nil.
func (s *State) Lookup(path string) *Entry {
	for i := range s.Entries {
		if s.Entries[i].Path == path {
			return &s.Entries[i]
		}
	}
	return nil
}

// Remove deletes all entries for the given groups. If groups is empty, all
// entries are removed.
func (s *State) Remove(groups []string) []Entry {
	if len(groups) == 0 {
		removed := s.Entries
		s.Entries = nil
		return removed
	}
	set := make(map[string]bool, len(groups))
	for _, g := range groups {
		set[g] = true
	}
	var kept, removed []Entry
	for _, e := range s.Entries {
		if set[e.Group] {
			removed = append(removed, e)
		} else {
			kept = append(kept, e)
		}
	}
	s.Entries = kept
	return removed
}

// Load reads state from path. If the file does not exist a blank State is
// returned without error.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s.Entries); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// Save writes state to path atomically.
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s.Entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}
