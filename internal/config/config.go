package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Mode controls how source paths are linked.
type Mode string

const (
	ModeLink    Mode = "link"    // direct symlink
	ModeTree    Mode = "tree"    // recursive/stow-style
	ModeEntries Mode = "entries" // link immediate children only
)

// LinkType controls what the symlink target path looks like.
type LinkType string

const (
	LinkTypeRel LinkType = "rel" // relative path (default)
	LinkTypeAbs LinkType = "abs" // absolute path
	LinkTypeRes LinkType = "res" // resolved (canonicalized) absolute path
)

// Flags holds the resolved flag values for a single link.
type Flags struct {
	Mode     Mode
	LinkType LinkType
	Dotfiles bool // rename dot- prefixed names to . in destination paths
}

// DefaultFlags returns the baseline defaults.
func DefaultFlags() Flags {
	return Flags{Mode: ModeLink, LinkType: LinkTypeRel}
}

// ParseFlags merges raw flag strings onto a base Flags value. The last flag in
// each group wins.
func ParseFlags(base Flags, raw []string) (Flags, error) {
	f := base
	for _, s := range raw {
		if s == "dotfiles" {
			f.Dotfiles = true
			continue
		}
		switch Mode(s) {
		case ModeLink, ModeTree, ModeEntries:
			f.Mode = Mode(s)
			continue
		}
		switch LinkType(s) {
		case LinkTypeRel, LinkTypeAbs, LinkTypeRes:
			f.LinkType = LinkType(s)
			continue
		}
		return f, fmt.Errorf("unknown flag %q", s)
	}
	return f, nil
}

// Link is a resolved link entry within a group.
type Link struct {
	Src   string // path relative to group source
	Dst   string // path relative to group target
	Flags Flags
}

// Group is a resolved group from the config.
type Group struct {
	Name      string
	Source    string
	Target    string
	CleanDirs bool // remove empty dirs under Target when cleaning
	Links     []Link
}

// Config is the parsed and resolved configuration.
type Config struct {
	Groups []Group
}

// ---- raw YAML types --------------------------------------------------------

type rawConfig map[string]rawGroup

type rawGroup struct {
	Source string     `yaml:"source"`
	Target string     `yaml:"target"`
	Flags  []string   `yaml:"flags"`
	Link   rawLinkMap `yaml:"link"`
}

// rawLinkMap is an ordered list of link entries, preserving YAML key order.
type rawLinkMap []rawLinkEntry

type rawLinkEntry struct {
	Key   string
	Value rawLinkTarget
}

func (m *rawLinkMap) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("link must be a mapping, got %v", value.Tag)
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		var rt rawLinkTarget
		if err := value.Content[i+1].Decode(&rt); err != nil {
			return err
		}
		*m = append(*m, rawLinkEntry{Key: value.Content[i].Value, Value: rt})
	}
	return nil
}

// rawLinkTarget can be a plain string or [target, flag, flag, ...]
type rawLinkTarget struct {
	Path  string
	Flags []string
}

func (r *rawLinkTarget) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		r.Path = value.Value
		return nil
	case yaml.SequenceNode:
		if len(value.Content) == 0 {
			return fmt.Errorf("link target sequence must not be empty")
		}
		r.Path = value.Content[0].Value
		for _, n := range value.Content[1:] {
			r.Flags = append(r.Flags, n.Value)
		}
		return nil
	default:
		return fmt.Errorf("link target must be a string or sequence, got %v", value.Tag)
	}
}

// ---- loading ---------------------------------------------------------------

// Load reads and parses the YAML config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg := &Config{}
	for name, rg := range raw {
		// Extract "clean_dirs" from flags before parsing mode/link-type flags.
		var cleanDirs bool
		var filteredFlags []string
		for _, f := range rg.Flags {
			if f == "clean_dirs" {
				cleanDirs = true
			} else {
				filteredFlags = append(filteredFlags, f)
			}
		}

		groupBase, err := ParseFlags(DefaultFlags(), filteredFlags)
		if err != nil {
			return nil, fmt.Errorf("group %q flags: %w", name, err)
		}

		g := Group{
			Name:      name,
			Source:    rg.Source,
			Target:    rg.Target,
			CleanDirs: cleanDirs,
		}

		for _, le := range rg.Link {
			src, rt := le.Key, le.Value
			linkFlags, err := ParseFlags(groupBase, rt.Flags)
			if err != nil {
				return nil, fmt.Errorf("group %q link %q flags: %w", name, src, err)
			}
			g.Links = append(g.Links, Link{
				Src:   src,
				Dst:   rt.Path,
				Flags: linkFlags,
			})
		}

		cfg.Groups = append(cfg.Groups, g)
	}

	return cfg, nil
}
