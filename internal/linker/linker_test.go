package linker

import (
	"os"
	"path/filepath"
	"testing"

	"lntab/internal/config"
	"lntab/internal/state"
)

// ---- helpers ---------------------------------------------------------------

func newState() *state.State { return &state.State{} }

// makeFile creates a regular file at path with the given content.
func makeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readLink returns the symlink target at path, failing the test if it is not a symlink.
func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	return target
}

func applyConfig(t *testing.T, cfg *config.Config, st *state.State, dryRun, verbose bool) error {
	t.Helper()
	return New(dryRun, verbose).Apply(cfg, nil, st)
}

// singleLinkConfig builds a minimal Config with one group containing one link.
func singleLinkConfig(src, dst string, flags config.Flags) *config.Config {
	return &config.Config{
		Groups: []config.Group{
			{
				Name:   "g",
				Source: "",
				Target: "",
				Links: []config.Link{
					{Src: src, Dst: dst, Flags: flags},
				},
			},
		},
	}
}

// ---- createLink conflict resolution ----------------------------------------

func TestCreateLink_NewLink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(false, false).createLink(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}

	got := readLink(t, dst)
	if got != src {
		t.Errorf("link target = %q, want %q", got, src)
	}
	var linkEntry *state.Entry
	for i := range st.Entries {
		if st.Entries[i].Kind == state.KindLink {
			linkEntry = &st.Entries[i]
		}
	}
	if linkEntry == nil {
		t.Fatal("no KindLink entry recorded in state")
	}
	if linkEntry.Target != src {
		t.Errorf("state target = %q, want %q", linkEntry.Target, src)
	}
}

func TestCreateLink_AlreadyCorrect(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create the correct symlink.
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(false, false).createLink(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}
	// State should not record a new entry because we did nothing.
	if len(st.Entries) != 0 {
		t.Errorf("state entries = %d, want 0", len(st.Entries))
	}
}

func TestCreateLink_ExistsNotSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")
	makeFile(t, dst, "i am a real file") // regular file, not a symlink

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	err := New(false, false).createLink(src, dst, flags, "g", newState())
	if err == nil {
		t.Fatal("expected error when dst is a regular file, got nil")
	}
}

func TestCreateLink_OverwriteOwnedSymlink(t *testing.T) {
	dir := t.TempDir()
	src1 := filepath.Join(dir, "src1", "file.txt")
	src2 := filepath.Join(dir, "src2", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src1, "v1")
	makeFile(t, src2, "v2")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a previous run: dst points to src1, recorded in state.
	if err := os.Symlink(src1, dst); err != nil {
		t.Fatal(err)
	}
	st := newState()
	st.Add(state.Entry{Kind: state.KindLink, Path: dst, Group: "g", Target: src1})

	// Now apply with src2 as the new target.
	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	if err := New(false, false).createLink(src2, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}
	got := readLink(t, dst)
	if got != src2 {
		t.Errorf("link target = %q, want %q", got, src2)
	}
	// State entry should be updated in place (still one entry).
	if len(st.Entries) != 1 {
		t.Errorf("state entries = %d, want 1", len(st.Entries))
	}
	if st.Entries[0].Target != src2 {
		t.Errorf("state target = %q, want %q", st.Entries[0].Target, src2)
	}
}

func TestCreateLink_ConflictNotOwnedSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	other := filepath.Join(dir, "other", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "src")
	makeFile(t, other, "other")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	// dst points to something not in state.
	if err := os.Symlink(other, dst); err != nil {
		t.Fatal(err)
	}

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	err := New(false, false).createLink(src, dst, flags, "g", newState())
	if err == nil {
		t.Fatal("expected error for unowned conflicting symlink, got nil")
	}
}

// ---- link modes ------------------------------------------------------------

func TestApplyMode_Tree(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	makeFile(t, filepath.Join(src, "a", "b.txt"), "b")
	makeFile(t, filepath.Join(src, "c.txt"), "c")

	flags := config.Flags{Mode: config.ModeTree, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(false, false).applyTree(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}

	// Files should be symlinked.
	readLink(t, filepath.Join(dst, "c.txt"))
	readLink(t, filepath.Join(dst, "a", "b.txt"))

	// Intermediate dir should be a real directory, not a symlink.
	info, err := os.Lstat(filepath.Join(dst, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("tree mode should create real dirs, not symlink them")
	}
}

func TestApplyMode_Entries(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	makeFile(t, filepath.Join(src, "file1.txt"), "1")
	makeFile(t, filepath.Join(src, "subdir", "nested.txt"), "n")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	flags := config.Flags{Mode: config.ModeEntries, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(false, false).applyEntries(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}

	// Direct children of src should be linked.
	readLink(t, filepath.Join(dst, "file1.txt"))
	readLink(t, filepath.Join(dst, "subdir"))

	// The nested file inside subdir should NOT be a separate symlink.
	_, err := os.Lstat(filepath.Join(dst, "subdir", "nested.txt"))
	if err != nil {
		t.Errorf("subdir/nested.txt inaccessible: %v", err)
	}
}

// ---- link types ------------------------------------------------------------

func TestLinkType_Relative(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeRel}
	st := newState()
	if err := New(false, false).createLink(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}
	target := readLink(t, dst)
	if filepath.IsAbs(target) {
		t.Errorf("rel link target should be relative, got %q", target)
	}
}

func TestLinkType_Absolute(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(false, false).createLink(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}
	target := readLink(t, dst)
	if !filepath.IsAbs(target) {
		t.Errorf("abs link target should be absolute, got %q", target)
	}
}

// ---- dry-run ---------------------------------------------------------------

func TestDryRun_NoFilesCreated(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")

	flags := config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
	st := newState()
	if err := New(true, false).createLink(src, dst, flags, "g", st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("dry-run should not create any files")
	}
	if len(st.Entries) != 0 {
		t.Error("dry-run should not record state entries")
	}
}

// ---- Clean -----------------------------------------------------------------

func TestClean_RemovesLinks(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	makeFile(t, src, "x")
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}

	st := newState()
	st.Add(state.Entry{Kind: state.KindLink, Path: dst, Group: "g", Target: src})

	if err := New(false, false).Clean(st, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("Clean should have removed the symlink")
	}
	if len(st.Entries) != 0 {
		t.Errorf("state entries = %d after clean, want 0", len(st.Entries))
	}
}

func TestClean_RemovesDirs(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "created")
	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}

	st := newState()
	st.Add(state.Entry{Kind: state.KindDir, Path: created, Group: "g"})

	if err := New(false, false).Clean(st, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(created); !os.IsNotExist(err) {
		t.Error("Clean should have removed the directory")
	}
}

func TestClean_FiltersByGroup(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	remove := filepath.Join(dir, "remove.txt")
	src := filepath.Join(dir, "src.txt")
	makeFile(t, src, "x")
	if err := os.Symlink(src, keep); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, remove); err != nil {
		t.Fatal(err)
	}

	st := newState()
	st.Add(state.Entry{Kind: state.KindLink, Path: keep, Group: "other", Target: src})
	st.Add(state.Entry{Kind: state.KindLink, Path: remove, Group: "g", Target: src})

	if err := New(false, false).Clean(st, []string{"g"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(remove); !os.IsNotExist(err) {
		t.Error("group g symlink should have been removed")
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Errorf("other group symlink should be kept: %v", err)
	}
}
