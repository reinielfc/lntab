package linker

import (
	"fmt"
	"os"
	"path/filepath"

	"lntab/internal/config"
	"lntab/internal/state"
)

// Linker carries dry-run and verbose options.
type Linker struct {
	dryRun  bool
	verbose bool
}

// New creates a Linker.
func New(dryRun, verbose bool) *Linker {
	return &Linker{dryRun: dryRun, verbose: verbose}
}

// Apply creates symlinks for the given groups (all groups if empty).
func (l *Linker) Apply(cfg *config.Config, groups []string, st *state.State) error {
	filter := groupSet(groups)
	for i := range cfg.Groups {
		g := &cfg.Groups[i]
		if len(filter) > 0 && !filter[g.Name] {
			continue
		}
		if err := l.applyGroup(g, st); err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
	}
	return nil
}

// Clean removes symlinks recorded in state for the given groups.
func (l *Linker) Clean(st *state.State, groups []string) error {
	removed := st.Remove(groups)
	// Remove symlinks first, then dirs (in reverse order so deepest first).
	var links, dirs []state.Entry
	for _, e := range removed {
		switch e.Kind {
		case state.KindLink:
			links = append(links, e)
		case state.KindDir:
			dirs = append(dirs, e)
		}
	}
	for _, e := range links {
		if l.verbose || l.dryRun {
			fmt.Printf("remove link %s\n", e.Path)
		}
		if !l.dryRun {
			if err := os.Remove(e.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", e.Path, err)
			}
		}
	}
	// Dirs in reverse order (deepest first).
	for i := len(dirs) - 1; i >= 0; i-- {
		e := dirs[i]
		if l.verbose || l.dryRun {
			fmt.Printf("rmdir %s\n", e.Path)
		}
		if !l.dryRun {
			// Only remove if empty.
			if err := os.Remove(e.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("rmdir %s: %w", e.Path, err)
			}
		}
	}
	return nil
}

// ---- group-level -----------------------------------------------------------

func (l *Linker) applyGroup(g *config.Group, st *state.State) error {
	for _, lnk := range g.Links {
		src := filepath.Join(g.Source, lnk.Src)
		dst := filepath.Join(g.Target, lnk.Dst)

		switch lnk.Flags.Mode {
		case config.ModeLink:
			if err := l.createLink(src, dst, lnk.Flags, g.Name, st); err != nil {
				return err
			}
		case config.ModeTree:
			if err := l.applyTree(src, dst, lnk.Flags, g.Name, st); err != nil {
				return err
			}
		case config.ModeEntries:
			if err := l.applyEntries(src, dst, lnk.Flags, g.Name, st); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown mode %q", lnk.Flags.Mode)
		}
	}
	return nil
}

// ---- mode implementations --------------------------------------------------

// applyTree mirrors the stow algorithm: for each file under src, create a
// symlink at the corresponding path under dst. Directories are created.
func (l *Linker) applyTree(src, dst string, flags config.Flags, group string, st *state.State) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			return l.ensureDir(target, group, st)
		}
		return l.createLink(path, target, flags, group, st)
	})
}

// applyEntries links the immediate children of src into dst.
func (l *Linker) applyEntries(src, dst string, flags config.Flags, group string, st *state.State) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", src, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if err := l.createLink(srcPath, dstPath, flags, group, st); err != nil {
			return err
		}
	}
	return nil
}

// ---- low-level helpers -----------------------------------------------------

// createLink creates a symlink at dst pointing to src according to flags.
func (l *Linker) createLink(src, dst string, flags config.Flags, group string, st *state.State) error {
	linkTarget, err := resolveLinkTarget(src, dst, flags.LinkType)
	if err != nil {
		return err
	}

	if l.verbose || l.dryRun {
		fmt.Printf("link %s -> %s\n", dst, linkTarget)
	}
	if l.dryRun {
		return nil
	}

	if err := l.ensureDir(filepath.Dir(dst), group, st); err != nil {
		return err
	}

	info, err := os.Lstat(dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dst, err)
	}

	if info != nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists and is not a symlink", dst)
		}
		current, err := os.Readlink(dst)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", dst, err)
		}
		if current == linkTarget {
			// Already pointing at the right target, nothing to do.
			return nil
		}
		// Allow overwrite only if the current target matches what lntab last wrote.
		prev := st.Lookup(dst)
		if prev == nil || prev.Target != current {
			return fmt.Errorf("%s is a symlink to %q which was not created by lntab (expected %q)", dst, current, linkTarget)
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove %s: %w", dst, err)
		}
		if err := os.Symlink(linkTarget, dst); err != nil {
			return fmt.Errorf("symlink %s -> %s: %w", dst, linkTarget, err)
		}
		// Update the existing state entry in place.
		prev.Target = linkTarget
		return nil
	}

	if err := os.Symlink(linkTarget, dst); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", dst, linkTarget, err)
	}
	st.Add(state.Entry{Kind: state.KindLink, Path: dst, Group: group, Target: linkTarget})
	return nil
}

func (l *Linker) ensureDir(dir string, group string, st *state.State) error {
	if l.dryRun {
		return nil
	}
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if l.verbose {
		fmt.Printf("mkdir %s\n", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	st.Add(state.Entry{Kind: state.KindDir, Path: dir, Group: group})
	return nil
}

// resolveLinkTarget returns the string that will be written into the symlink.
func resolveLinkTarget(src, dst string, lt config.LinkType) (string, error) {
	switch lt {
	case config.LinkTypeAbs:
		abs, err := filepath.Abs(src)
		if err != nil {
			return "", fmt.Errorf("abs(%s): %w", src, err)
		}
		return abs, nil
	case config.LinkTypeRes:
		res, err := filepath.EvalSymlinks(src)
		if err != nil {
			return "", fmt.Errorf("resolve(%s): %w", src, err)
		}
		return res, nil
	default: // LinkTypeRel
		dstDir := filepath.Dir(dst)
		rel, err := filepath.Rel(dstDir, src)
		if err != nil {
			return "", fmt.Errorf("rel(%s, %s): %w", dstDir, src, err)
		}
		return rel, nil
	}
}

// groupSet builds a name→true map for fast membership checks.
func groupSet(groups []string) map[string]bool {
	if len(groups) == 0 {
		return nil
	}
	m := make(map[string]bool, len(groups))
	for _, g := range groups {
		m[g] = true
	}
	return m
}
