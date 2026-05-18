package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- ParseFlags ------------------------------------------------------------

func TestParseFlags_Defaults(t *testing.T) {
	f, err := ParseFlags(DefaultFlags(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != ModeLink {
		t.Errorf("mode = %q, want %q", f.Mode, ModeLink)
	}
	if f.LinkType != LinkTypeRel {
		t.Errorf("link-type = %q, want %q", f.LinkType, LinkTypeRel)
	}
}

func TestParseFlags_ModeOverride(t *testing.T) {
	f, err := ParseFlags(DefaultFlags(), []string{"tree"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != ModeTree {
		t.Errorf("mode = %q, want %q", f.Mode, ModeTree)
	}
}

func TestParseFlags_LinkTypeOverride(t *testing.T) {
	f, err := ParseFlags(DefaultFlags(), []string{"abs"})
	if err != nil {
		t.Fatal(err)
	}
	if f.LinkType != LinkTypeAbs {
		t.Errorf("link-type = %q, want %q", f.LinkType, LinkTypeAbs)
	}
}

func TestParseFlags_LastWins(t *testing.T) {
	// Two mode flags — last one wins.
	f, err := ParseFlags(DefaultFlags(), []string{"tree", "entries"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != ModeEntries {
		t.Errorf("mode = %q, want %q", f.Mode, ModeEntries)
	}
}

func TestParseFlags_BasePreserved(t *testing.T) {
	// Only override mode; link-type should keep the base value.
	base := Flags{Mode: ModeLink, LinkType: LinkTypeAbs}
	f, err := ParseFlags(base, []string{"tree"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != ModeTree {
		t.Errorf("mode = %q, want %q", f.Mode, ModeTree)
	}
	if f.LinkType != LinkTypeAbs {
		t.Errorf("link-type = %q, want %q", f.LinkType, LinkTypeAbs)
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	_, err := ParseFlags(DefaultFlags(), []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

// ---- Load ------------------------------------------------------------------

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "lntab.yml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_StringTarget(t *testing.T) {
	p := writeConfig(t, `
mygroup:
  source: /src
  target: /dst
  flags: [abs]
  link:
    foo: bar
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(cfg.Groups))
	}
	g := cfg.Groups[0]
	if g.Name != "mygroup" {
		t.Errorf("name = %q, want %q", g.Name, "mygroup")
	}
	if g.Source != "/src" {
		t.Errorf("source = %q, want %q", g.Source, "/src")
	}
	if len(g.Links) != 1 {
		t.Fatalf("links = %d, want 1", len(g.Links))
	}
	lnk := g.Links[0]
	if lnk.Src != "foo" {
		t.Errorf("src = %q, want %q", lnk.Src, "foo")
	}
	if lnk.Dst != "bar" {
		t.Errorf("dst = %q, want %q", lnk.Dst, "bar")
	}
	// Group flag abs should be inherited.
	if lnk.Flags.LinkType != LinkTypeAbs {
		t.Errorf("link-type = %q, want %q", lnk.Flags.LinkType, LinkTypeAbs)
	}
}

func TestLoad_SequenceTargetWithFlagOverride(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  flags: [link, rel]
  link:
    a: [b, tree, abs]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	lnk := cfg.Groups[0].Links[0]
	if lnk.Dst != "b" {
		t.Errorf("dst = %q, want %q", lnk.Dst, "b")
	}
	if lnk.Flags.Mode != ModeTree {
		t.Errorf("mode = %q, want %q", lnk.Flags.Mode, ModeTree)
	}
	if lnk.Flags.LinkType != LinkTypeAbs {
		t.Errorf("link-type = %q, want %q", lnk.Flags.LinkType, LinkTypeAbs)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/lntab.yml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// A tab character in a flow sequence is invalid YAML syntax.
	p := writeConfig(t, "key: [unclosed")
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_UnknownFlag(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  flags: [badmode]
  link:
    a: b
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

func TestLoad_EmptySequenceTarget(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  link:
    a: []
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for empty sequence target, got nil")
	}
}

func TestLoad_LinkOrder(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  link:
    first: a
    second: b
    third: c
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	links := cfg.Groups[0].Links
	if len(links) != 3 {
		t.Fatalf("links = %d, want 3", len(links))
	}
	want := []string{"first", "second", "third"}
	for i, lnk := range links {
		if lnk.Src != want[i] {
			t.Errorf("links[%d].Src = %q, want %q", i, lnk.Src, want[i])
		}
	}
}

func TestLoad_CleanDirsFlag(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  flags: [abs, clean_dirs]
  link:
    a: b
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	g := cfg.Groups[0]
	if !g.CleanDirs {
		t.Error("CleanDirs = false, want true")
	}
	// Other flags should still parse correctly.
	if g.Links[0].Flags.LinkType != LinkTypeAbs {
		t.Errorf("link-type = %q, want abs", g.Links[0].Flags.LinkType)
	}
}

func TestLoad_CleanDirsDefault(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  link:
    a: b
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Groups[0].CleanDirs {
		t.Error("CleanDirs should default to false")
	}
}

func TestParseFlags_Overwrite(t *testing.T) {
	f, err := ParseFlags(DefaultFlags(), []string{"overwrite"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.Overwrite {
		t.Error("Overwrite = false, want true")
	}
}

func TestLoad_OverwriteFlag(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  flags: [overwrite]
  link:
    a: b
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Groups[0].Links[0].Flags.Overwrite {
		t.Error("Overwrite = false, want true")
	}
}

func TestParseFlags_Dotfiles(t *testing.T) {
	f, err := ParseFlags(DefaultFlags(), []string{"dotfiles"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.Dotfiles {
		t.Error("Dotfiles = false, want true")
	}
}

func TestLoad_DotfilesFlag(t *testing.T) {
	p := writeConfig(t, `
g:
  source: /s
  target: /t
  flags: [dotfiles]
  link:
    a: b
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Groups[0].Links[0].Flags.Dotfiles {
		t.Error("Dotfiles = false, want true")
	}
}

func TestLoad_MultipleGroups(t *testing.T) {
	p := writeConfig(t, `
alpha:
  source: /a
  target: /ta
  link:
    x: y
beta:
  source: /b
  target: /tb
  link:
    p: q
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(cfg.Groups))
	}
}
