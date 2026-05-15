package state

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- Load / Save -----------------------------------------------------------

func TestLoad_MissingFile(t *testing.T) {
	st, err := Load(filepath.Join(t.TempDir(), "missing.state"))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(st.Entries))
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.state")
	data := `[{"kind":"link","path":"/a/b","group":"g","target":"/x/y"}]`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(st.Entries))
	}
	e := st.Entries[0]
	if e.Kind != KindLink {
		t.Errorf("kind = %q, want %q", e.Kind, KindLink)
	}
	if e.Path != "/a/b" {
		t.Errorf("path = %q, want %q", e.Path, "/a/b")
	}
	if e.Target != "/x/y" {
		t.Errorf("target = %q, want %q", e.Target, "/x/y")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.state")
	if err := os.WriteFile(p, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "round.state")
	st := &State{}
	st.Add(Entry{Kind: KindDir, Path: "/a", Group: "g1"})
	st.Add(Entry{Kind: KindLink, Path: "/a/f", Group: "g1", Target: "/src/f"})

	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	st2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(st2.Entries))
	}
	if st2.Entries[1].Target != "/src/f" {
		t.Errorf("target = %q, want %q", st2.Entries[1].Target, "/src/f")
	}
}

func TestSave_Atomic(t *testing.T) {
	// Saving must not leave a .tmp file behind.
	dir := t.TempDir()
	p := filepath.Join(dir, "test.state")
	st := &State{}
	if err := st.Save(p); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "test.state" {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

// ---- Lookup ----------------------------------------------------------------

func TestLookup_Found(t *testing.T) {
	st := &State{}
	st.Add(Entry{Kind: KindLink, Path: "/a/b", Group: "g", Target: "/s/b"})
	e := st.Lookup("/a/b")
	if e == nil {
		t.Fatal("Lookup returned nil, want entry")
	}
	if e.Target != "/s/b" {
		t.Errorf("target = %q, want %q", e.Target, "/s/b")
	}
}

func TestLookup_NotFound(t *testing.T) {
	st := &State{}
	if got := st.Lookup("/no/such"); got != nil {
		t.Errorf("Lookup = %v, want nil", got)
	}
}

func TestLookup_ReturnsPointer(t *testing.T) {
	// Mutations via the pointer must be visible in the state.
	st := &State{}
	st.Add(Entry{Kind: KindLink, Path: "/p", Group: "g", Target: "old"})
	e := st.Lookup("/p")
	e.Target = "new"
	if st.Entries[0].Target != "new" {
		t.Errorf("mutation not reflected; target = %q, want %q", st.Entries[0].Target, "new")
	}
}

// ---- Remove ----------------------------------------------------------------

func TestRemove_ByGroup(t *testing.T) {
	st := &State{}
	st.Add(Entry{Kind: KindLink, Path: "/a", Group: "g1"})
	st.Add(Entry{Kind: KindLink, Path: "/b", Group: "g2"})
	st.Add(Entry{Kind: KindLink, Path: "/c", Group: "g1"})

	removed := st.Remove([]string{"g1"})
	if len(removed) != 2 {
		t.Errorf("removed = %d, want 2", len(removed))
	}
	if len(st.Entries) != 1 {
		t.Errorf("remaining = %d, want 1", len(st.Entries))
	}
	if st.Entries[0].Path != "/b" {
		t.Errorf("remaining path = %q, want %q", st.Entries[0].Path, "/b")
	}
}

func TestRemove_AllGroups(t *testing.T) {
	st := &State{}
	st.Add(Entry{Kind: KindLink, Path: "/a", Group: "g1"})
	st.Add(Entry{Kind: KindLink, Path: "/b", Group: "g2"})

	removed := st.Remove(nil)
	if len(removed) != 2 {
		t.Errorf("removed = %d, want 2", len(removed))
	}
	if len(st.Entries) != 0 {
		t.Errorf("remaining = %d, want 0", len(st.Entries))
	}
}

func TestRemove_UnknownGroup(t *testing.T) {
	st := &State{}
	st.Add(Entry{Kind: KindLink, Path: "/a", Group: "g1"})

	removed := st.Remove([]string{"nope"})
	if len(removed) != 0 {
		t.Errorf("removed = %d, want 0", len(removed))
	}
	if len(st.Entries) != 1 {
		t.Errorf("remaining = %d, want 1", len(st.Entries))
	}
}
