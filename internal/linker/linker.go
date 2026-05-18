package linker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/reinielfc/lntab/internal/config"
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
func (l *Linker) Apply(cfg *config.Config, groups []string) error {
	filter := groupSet(groups)
	for i := range cfg.Groups {
		g := &cfg.Groups[i]
		if len(filter) > 0 && !filter[g.Name] {
			continue
		}
		if err := l.applyGroup(g); err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
	}
	return nil
}

// Clean removes symlinks defined in config for the given groups. When a
// group has CleanDirs set, empty directories left behind under the group
// target are also removed (deepest first, never the target root itself).
func (l *Linker) Clean(cfg *config.Config, groups []string) error {
	filter := groupSet(groups)
	for i := range cfg.Groups {
		g := &cfg.Groups[i]
		if len(filter) > 0 && !filter[g.Name] {
			continue
		}
		if err := l.cleanGroup(g); err != nil {
			return fmt.Errorf("group %q: %w", g.Name, err)
		}
	}
	return nil
}

// ---- group-level -----------------------------------------------------------

func (l *Linker) applyGroup(g *config.Group) error {
	for _, lnk := range g.Links {
		src := filepath.Join(g.Source, lnk.Src)
		dst := filepath.Join(g.Target, lnk.Dst)

		switch lnk.Flags.Mode {
		case config.ModeLink:
			if err := l.createLink(src, dst, lnk.Flags); err != nil {
				return err
			}
		case config.ModeTree:
			if err := l.applyTree(src, dst, lnk.Flags); err != nil {
				return err
			}
		case config.ModeEntries:
			if err := l.applyEntries(src, dst, lnk.Flags); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown mode %q", lnk.Flags.Mode)
		}
	}
	return nil
}

func (l *Linker) cleanGroup(g *config.Group) error {
	// Collect all candidate symlink paths first, then sort deepest-first so
	// that a nested path is always removed before its parent. This prevents
	// a parent removal making child paths unreachable via os.Lstat.
	var candidates []string

	for _, lnk := range g.Links {
		src := filepath.Join(g.Source, lnk.Src)
		dst := filepath.Join(g.Target, lnk.Dst)

		switch lnk.Flags.Mode {
		case config.ModeLink:
			candidates = append(candidates, dst)
		case config.ModeTree:
			paths, err := l.collectTreeFlags(src, dst, lnk.Flags)
			if err != nil {
				return err
			}
			candidates = append(candidates, paths...)
		case config.ModeEntries:
			paths, err := l.collectEntriesFlags(src, dst, lnk.Flags)
			if err != nil {
				return err
			}
			candidates = append(candidates, paths...)
		default:
			return fmt.Errorf("unknown mode %q", lnk.Flags.Mode)
		}
	}

	// Deepest paths first (longer path → deeper in tree).
	sort.Slice(candidates, func(i, j int) bool { return len(candidates[i]) > len(candidates[j]) })

	var removed []string
	for _, dst := range candidates {
		if r, err := l.removeLink(dst); err != nil {
			return err
		} else if r {
			removed = append(removed, dst)
		}
	}

	if g.CleanDirs {
		if err := l.pruneEmptyDirs(g.Target, removed); err != nil {
			return err
		}
	}

	return nil
}

// ---- mode implementations --------------------------------------------------

// applyTree mirrors the stow algorithm: for each file under src, create a
// symlink at the corresponding path under dst. Directories are created.
func (l *Linker) applyTree(src, dst string, flags config.Flags) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstRel := rel
		if flags.Dotfiles {
			dstRel = dotfilePath(rel)
		}
		target := filepath.Join(dst, dstRel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			return l.ensureDir(target)
		}
		return l.createLink(path, target, flags)
	})
}

// applyEntries links the immediate children of src into dst.
func (l *Linker) applyEntries(src, dst string, flags config.Flags) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", src, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstName := e.Name()
		if flags.Dotfiles {
			dstName = dotfileName(dstName)
		}
		dstPath := filepath.Join(dst, dstName)
		if err := l.createLink(srcPath, dstPath, flags); err != nil {
			return err
		}
	}
	return nil
}

// collectTree returns dst paths corresponding to files under src, without removing anything.
func (l *Linker) collectTree(src, dst string) ([]string, error) {
	return l.collectTreeFlags(src, dst, config.Flags{})
}

func (l *Linker) collectTreeFlags(src, dst string, flags config.Flags) ([]string, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil, nil
	}
	var paths []string
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstRel := rel
		if flags.Dotfiles {
			dstRel = dotfilePath(rel)
		}
		paths = append(paths, filepath.Join(dst, dstRel))
		return nil
	})
	return paths, err
}

// collectEntries returns dst paths for each immediate child of src, without removing anything.
func (l *Linker) collectEntries(src, dst string) ([]string, error) {
	return l.collectEntriesFlags(src, dst, config.Flags{})
}

func (l *Linker) collectEntriesFlags(src, dst string, flags config.Flags) ([]string, error) {
	entries, err := os.ReadDir(src)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", src, err)
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if flags.Dotfiles {
			name = dotfileName(name)
		}
		paths = append(paths, filepath.Join(dst, name))
	}
	return paths, nil
}

// pruneEmptyDirs removes empty directories that are strict descendants of root
// and became empty after the symlinks at removed paths were deleted. Dirs are
// processed deepest-first so a parent is only removed once its children are gone.
func (l *Linker) pruneEmptyDirs(root string, removed []string) error {
	seen := make(map[string]bool)
	var dirs []string
	for _, p := range removed {
		dir := filepath.Dir(p)
		for {
			rel, err := filepath.Rel(root, dir)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				break
			}
			if seen[dir] {
				break
			}
			seen[dir] = true
			dirs = append(dirs, dir)
			dir = filepath.Dir(dir)
		}
	}
	// Deepest paths first (longest string length).
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			continue
		}
		if l.verbose || l.dryRun {
			fmt.Printf("rmdir %s\n", dir)
		}
		if l.dryRun {
			continue
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rmdir %s: %w", dir, err)
		}
	}
	return nil
}

// ---- low-level helpers -----------------------------------------------------

// createLink creates a symlink at dst pointing to src according to flags.
func (l *Linker) createLink(src, dst string, flags config.Flags) error {
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

	if err := l.ensureDir(filepath.Dir(dst)); err != nil {
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
			return nil
		}
		if !flags.Overwrite {
			return fmt.Errorf("%s is a symlink to %q, expected %q", dst, current, linkTarget)
		}
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove %s: %w", dst, err)
		}
	}

	if err := os.Symlink(linkTarget, dst); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", dst, linkTarget, err)
	}
	return nil
}

func (l *Linker) ensureDir(dir string) error {
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
	return nil
}

// removeLink removes the symlink at dst if it exists. Returns true if removed.
// Regular files and directories are left untouched.
func (l *Linker) removeLink(dst string) (bool, error) {
	info, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", dst, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	if l.verbose || l.dryRun {
		fmt.Printf("remove link %s\n", dst)
	}
	if l.dryRun {
		return true, nil
	}
	if err := os.Remove(dst); err != nil {
		return false, fmt.Errorf("remove %s: %w", dst, err)
	}
	return true, nil
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

// dotfileName replaces a "dot-" prefix with "." for a single path component.
func dotfileName(name string) string {
	if strings.HasPrefix(name, "dot-") {
		return "." + name[4:]
	}
	return name
}

// dotfilePath applies dotfileName to every component of a slash-separated path.
func dotfilePath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		parts[i] = dotfileName(p)
	}
	return filepath.Join(parts...)
}
