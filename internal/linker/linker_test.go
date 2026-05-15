package linker

import (
	"os"
	"path/filepath"
	"testing"

	"lntab/internal/config"
)

// ---- helpers ---------------------------------------------------------------

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

// groupConfig builds a Config with one group.
func groupConfig(name, source, target string, cleanDirs bool, links ...config.Link) *config.Config {
	return &config.Config{
		Groups: []config.Group{
			{Name: name, Source: source, Target: target, CleanDirs: cleanDirs, Links: links},
		},
	}
}

func link(src, dst string, flags config.Flags) config.Link {
	return config.Link{Src: src, Dst: dst, Flags: flags}
}

var flagsAbs = config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeAbs}
var flagsRel = config.Flags{Mode: config.ModeLink, LinkType: config.LinkTypeRel}

// ---- createLink conflict resolution ----------------------------------------

func TestCreateLink_NewLink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")

	if err := New(false, false).createLink(src, dst, flagsAbs); err != nil {
		t.Fatal(err)
	}
	got := readLink(t, dst)
	if got != src {
		t.Errorf("link target = %q, want %q", got, src)
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
	if err := os.Symlink(src, dst); err != nil {
		t.Fatal(err)
	}

	// Applying again should be a no-op.
	if err := New(false, false).createLink(src, dst, flagsAbs); err != nil {
		t.Fatal(err)
	}
	if got := readLink(t, dst); got != src {
		t.Errorf("link target = %q, want %q", got, src)
	}
}

func TestCreateLink_ExistsNotSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "hello")
	makeFile(t, dst, "i am a real file")

	err := New(false, false).createLink(src, dst, flagsAbs)
	if err == nil {
		t.Fatal("expected error when dst is a regular file, got nil")
	}
}

func TestCreateLink_ConflictSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "file.txt")
	other := filepath.Join(dir, "other", "file.txt")
	dst := filepath.Join(dir, "dst", "file.txt")
	makeFile(t, src, "src")
	makeFile(t, other, "other")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, dst); err != nil {
		t.Fatal(err)
	}

	err := New(false, false).createLink(src, dst, flagsAbs)
	if err == nil {
		t.Fatal("expected error for conflicting symlink, got nil")
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
	if err := New(false, false).applyTree(src, dst, flags); err != nil {
		t.Fatal(err)
	}

	readLink(t, filepath.Join(dst, "c.txt"))
	readLink(t, filepath.Join(dst, "a", "b.txt"))

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
	if err := New(false, false).applyEntries(src, dst, flags); err != nil {
		t.Fatal(err)
	}

	readLink(t, filepath.Join(dst, "file1.txt"))
	readLink(t, filepath.Join(dst, "subdir"))

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

	if err := New(false, false).createLink(src, dst, flagsRel); err != nil {
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

	if err := New(false, false).createLink(src, dst, flagsAbs); err != nil {
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

	if err := New(true, false).createLink(src, dst, flagsAbs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("dry-run should not create any files")
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

	cfg := groupConfig("g", dir, dir, false, link("src.txt", "dst.txt", flagsAbs))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("Clean should have removed the symlink")
	}
}

func TestClean_SkipsNonSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	makeFile(t, real, "i am real")

	cfg := groupConfig("g", dir, dir, false, link("real.txt", "real.txt", flagsAbs))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}
	// Regular file must not be removed.
	if _, err := os.Lstat(real); err != nil {
		t.Errorf("real file should be preserved: %v", err)
	}
}

func TestClean_FiltersByGroup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	keepDst := filepath.Join(dir, "keep.txt")
	removeDst := filepath.Join(dir, "remove.txt")
	makeFile(t, src, "x")
	if err := os.Symlink(src, keepDst); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, removeDst); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Groups: []config.Group{
			{Name: "keep", Source: dir, Target: dir, Links: []config.Link{link("src.txt", "keep.txt", flagsAbs)}},
			{Name: "remove", Source: dir, Target: dir, Links: []config.Link{link("src.txt", "remove.txt", flagsAbs)}},
		},
	}
	if err := New(false, false).Clean(cfg, []string{"remove"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(removeDst); !os.IsNotExist(err) {
		t.Error("group 'remove' symlink should have been removed")
	}
	if _, err := os.Lstat(keepDst); err != nil {
		t.Errorf("group 'keep' symlink should be preserved: %v", err)
	}
}

func TestClean_Tree_RemovesLinks(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	makeFile(t, filepath.Join(src, "a", "b.txt"), "b")
	makeFile(t, filepath.Join(src, "c.txt"), "c")

	// Apply first to create the symlinks.
	treeFlags := config.Flags{Mode: config.ModeTree, LinkType: config.LinkTypeAbs}
	if err := New(false, false).applyTree(src, dst, treeFlags); err != nil {
		t.Fatal(err)
	}

	cfg := groupConfig("g", dir, dir, false, link("src", "dst", treeFlags))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(dst, "c.txt")); !os.IsNotExist(err) {
		t.Error("c.txt symlink should be removed")
	}
	if _, err := os.Lstat(filepath.Join(dst, "a", "b.txt")); !os.IsNotExist(err) {
		t.Error("a/b.txt symlink should be removed")
	}
}

func TestClean_Entries_RemovesLinks(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	makeFile(t, filepath.Join(src, "file1.txt"), "1")
	makeFile(t, filepath.Join(src, "file2.txt"), "2")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	entFlags := config.Flags{Mode: config.ModeEntries, LinkType: config.LinkTypeAbs}
	if err := New(false, false).applyEntries(src, dst, entFlags); err != nil {
		t.Fatal(err)
	}

	cfg := groupConfig("g", dir, dir, false, link("src", "dst", entFlags))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(dst, "file1.txt")); !os.IsNotExist(err) {
		t.Error("file1.txt symlink should be removed")
	}
}

func TestClean_CleanDirs_RemovesEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	target := filepath.Join(dir, "target")

	makeFile(t, filepath.Join(src, "sub", "file.txt"), "x")

	treeFlags := config.Flags{Mode: config.ModeTree, LinkType: config.LinkTypeAbs}
	if err := New(false, false).applyTree(src, target, treeFlags); err != nil {
		t.Fatal(err)
	}
	// target/sub/file.txt is a symlink; target/sub is a real dir.
	if _, err := os.Lstat(filepath.Join(target, "sub")); err != nil {
		t.Fatal(err)
	}

	cfg := groupConfig("g", dir, target, true, link("src", ".", treeFlags))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}

	// Symlink removed.
	if _, err := os.Lstat(filepath.Join(target, "src", "sub", "file.txt")); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}
	// target itself must still exist.
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("group target dir should be preserved: %v", err)
	}
}

func TestClean_CleanDirs_PreservesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	target := filepath.Join(dir, "target")

	makeFile(t, filepath.Join(src, "managed.txt"), "managed")
	// An extra file that lntab does not manage.
	extra := filepath.Join(target, "extra.txt")
	makeFile(t, extra, "extra")

	treeFlags := config.Flags{Mode: config.ModeTree, LinkType: config.LinkTypeAbs}
	if err := New(false, false).applyTree(src, target, treeFlags); err != nil {
		t.Fatal(err)
	}

	cfg := groupConfig("g", dir, target, true, link("src", ".", treeFlags))
	if err := New(false, false).Clean(cfg, nil); err != nil {
		t.Fatal(err)
	}

	// The non-empty target dir must survive.
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("target dir should be preserved (it still has extra.txt): %v", err)
	}
	if _, err := os.Lstat(extra); err != nil {
		t.Errorf("extra.txt should be untouched: %v", err)
	}
}
