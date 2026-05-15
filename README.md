# lntab

A declarative symlink manager. Define groups of symlinks in a YAML config file and apply or remove them with a single command.

## Installation

```sh
go install github.com/reinielfc/lntab@latest
```

Or clone and install locally:

```sh
git clone https://github.com/reinielfc/lntab
cd lntab
go install .
```

## Usage

```
lntab [-config <path>] [-n] [-v] <command> [groups...]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `-config` | | `lntab.yml` | Path to the config file |
| `-dry-run` | `-n` | false | Print actions without executing them |
| `-verbose` | `-v` | false | Print each created symlink |

### Commands

| Command | Description |
|---------|-------------|
| `apply` | Create or update symlinks defined in the config |
| `clean` | Remove symlinks previously created by lntab |

Optionally pass one or more group names to act on a subset:

```sh
lntab apply home nested
lntab clean home
```

## Config Format

```yaml
<group_name>:
  source: <source_directory>
  target: <target_directory>
  flags: [<flag>, ...]       # group-level defaults
  link:
    <source_subpath>: <target_subpath>
    <source_subpath>: [<target_subpath>, <flag>, ...]  # per-link overrides
```

Per-link flags are merged on top of the group defaults. Within each flag group the last value wins.

### Mode flags

| Flag | Description |
|------|-------------|
| `link` | (default) Symlink `source/src` → `target/dst` directly |
| `tree` | Stow-style: mirror every file under `src` into `dst`, creating real directories |
| `entries` | Link only the immediate children of `src` into `dst` |

### Link-type flags

| Flag | Description |
|------|-------------|
| `rel` | (default) Symlink target is a relative path |
| `abs` | Symlink target is an absolute path |
| `res` | Symlink target is a canonicalized (resolved) absolute path |

## Example

```yaml
home:
  source: /mnt/data/dotfiles
  target: /home/user
  flags: [link, rel]
  link:
    config/nvim:  .config/nvim
    config/zsh:   .config/zsh
    config/git:   .config/git

media:
  source: /mnt/data
  target: /home/user
  flags: [link, abs]
  link:
    movies: Videos/Movies
    music:  Music
```

Apply everything:

```sh
lntab apply
```

Dry-run a single group:

```sh
lntab -n -v apply home
```

Remove all managed symlinks:

```sh
lntab clean
```
