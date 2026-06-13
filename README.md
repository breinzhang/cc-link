# cc-link

`cc-link` is a small Go CLI that links a shared Claude Code asset library into any project `.claude/` directory.

It supports `skills`, `agents`, `commands`, `rules`, and any other top-level asset kind in your source library. Skills are special-cased because they use a two-level layout; other kinds are mapped as-is.

## Install

```bash
go install github.com/breinzhang/cc-link@latest
```

Local development:

```bash
go install .
```

## Source Layout

```text
src
├── agents
│   └── reviewer.md
├── commands
│   └── git/commit.md
├── rules
│   └── memory.md
└── skills
    ├── custom
    │   └── my-skill
    └── superpowers
        └── test-driven-development
```

## Initialize

Run once to create your user defaults:

```bash
cc-link init --src "/path/to/src"
```

This writes `~/.cc-link/cc-link.json`:

```json
{
  "sourceRoot": "/path/to/src",
  "targetRoot": ".claude",
  "statusline": {
    "lines": 2,
    "cost": {
      "enabled": true,
      "currency": "USD",
      "prices": [
        {
          "provider": "Kimi/Moonshot",
          "models": [
            {
              "match": "kimi-k2.7-code*",
              "input": 0.95,
              "output": 4.0,
              "cacheWrite": 0.95,
              "cacheRead": 0.19
            }
          ]
        },
        {
          "provider": "GLM/Z.AI",
          "models": [
            {
              "match": "glm-5.2*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            },
            {
              "match": "glm-5.1*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            }
          ]
        },
        {
          "provider": "DeepSeek",
          "models": [
            {
              "match": "deepseek-v4-flash*",
              "input": 0.14,
              "output": 0.28,
              "cacheWrite": 0.14,
              "cacheRead": 0.0028
            }
          ]
        },
        {
          "provider": "MiniMax",
          "models": [
            {
              "match": "minimax-m3*",
              "input": 0.30,
              "output": 1.20,
              "cacheWrite": 0.30,
              "cacheRead": 0.06
            }
          ]
        }
      ]
    }
  }
}
```

`init` writes editable USD-per-1M-token price presets for direct model-vendor APIs, currently Kimi/Moonshot, GLM/Z.AI, DeepSeek, and MiniMax. The example above is shortened; the generated config includes multiple Kimi K2, GLM 5.x/4.x, DeepSeek V4, and MiniMax M-series variants. GLM 5.2 is preset to the GLM 5.1 price until Z.AI publishes a separate API price.

Enable the Claude Code statusline at the same time:

```bash
cc-link init --src "/path/to/src" --statusline
```

## Link Assets

From a project root:

```bash
cc-link link skills custom
cc-link link skills custom/my-skill
cc-link link agents reviewer.md
cc-link link commands git/commit.md
cc-link link rules memory.md
cc-link link all
```

`link` writes or merges `.cc-link/cc-link.json` in the project and writes `.claude/.cc-link.lock.json`. If `--src` is provided to `link`, that source root is stored in the project config so later `apply` can run without `--src`.

Skills behavior:

- `skills <category>` links every skill under that category into `.claude/skills/<skill>`.
- `skills <category>/<skill>` links one skill.
- `skills <skill>` searches all categories; if multiple categories contain the same skill name, use `category/skill`.

Other kinds map directly:

```text
src/rules/memory.md -> .claude/rules/memory.md
src/agents/reviewer.md -> .claude/agents/reviewer.md
```

## Apply Config

Project `.cc-link/cc-link.json`:

```json
{
  "sourceRoot": "/path/to/src",
  "targetRoot": ".claude",
  "statusline": {
    "lines": 2,
    "cost": {
      "enabled": true,
      "currency": "USD",
      "prices": [
        {
          "provider": "GLM/Z.AI",
          "models": [
            {
              "match": "glm-5.2*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            },
            {
              "match": "glm-5.1*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            }
          ]
        },
        {
          "provider": "Kimi/Moonshot",
          "models": [
            {
              "match": "kimi-k2.6*",
              "input": 0.95,
              "output": 4.0,
              "cacheWrite": 0.95,
              "cacheRead": 0.16
            }
          ]
        },
        {
          "provider": "DeepSeek",
          "models": [
            {
              "match": "deepseek-v4-pro*",
              "input": 0.435,
              "output": 0.87,
              "cacheWrite": 0.435,
              "cacheRead": 0.003625
            }
          ]
        },
        {
          "provider": "MiniMax",
          "models": [
            {
              "match": "minimax-m2.7*",
              "input": 0.30,
              "output": 1.20,
              "cacheWrite": 0.375,
              "cacheRead": 0.06
            }
          ]
        }
      ]
    }
  },
  "links": {
    "skills": ["custom", "superpowers/test-driven-development"],
    "agents": ["reviewer.md"],
    "commands": ["git/commit.md"],
    "rules": ["memory.md"]
  }
}
```

Apply it:

```bash
cc-link apply
```

Zero-config behavior:

```bash
cc-link apply --src "/path/to/src"
```

`apply` reads `~/.cc-link/cc-link.json` first, then overlays project `.cc-link/cc-link.json`. Project settings have higher priority.

If the effective config has no `links`, or has an empty `links` map, `apply` links every top-level kind from `sourceRoot`.

If neither global nor project config provides `sourceRoot`, pass `--src`; `apply` links every top-level kind from that source library and writes only the lock file. It does not create project config.

Prune links that were created by this tool but are no longer in config:

```bash
cc-link apply --prune
```

## Unlink And Status

```bash
cc-link unlink skills custom
cc-link unlink skills --all
cc-link unlink rules memory.md
cc-link unlink all --all
cc-link status
```

`unlink` and `prune` remove only entries recorded in `.claude/.cc-link.lock.json`.

## Windows

On macOS and Linux, `cc-link` uses symlinks.

On Windows, it avoids the usual symlink privilege problem when possible:

- directories use NTFS junctions via `mklink /J`;
- files use hardlinks via `os.Link`;
- if those fail, it falls back to symlinks, which may require Developer Mode or Administrator.

Windows-specific junction/hardlink behavior should be validated on a Windows machine.

## Statusline

Enable globally:

```bash
cc-link statusline enable
```

Disable:

```bash
cc-link statusline disable
```

Claude Code will run:

```bash
cc-link statusline
```

`statusline enable` writes the command as `cc-link statusline`, so `cc-link` must be available in the `PATH` used by Claude Code. The command reads Claude Code status JSON from stdin and renders a configurable statusline with model, directory, git branch, current time, context usage, prompt-cache countdown, and cost summary.

Configure it in `~/.cc-link/cc-link.json` or project `.cc-link/cc-link.json`:

```json
{
  "statusline": {
    "lines": 2,
    "cost": {
      "enabled": true,
      "currency": "USD",
      "prices": [
        {
          "provider": "GLM/Z.AI",
          "models": [
            {
              "match": "glm-5.2*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            },
            {
              "match": "glm-5.1*",
              "input": 1.4,
              "output": 4.4,
              "cacheWrite": 1.4,
              "cacheRead": 0.26
            }
          ]
        }
      ]
    }
  }
}
```

`lines` defaults to `2`. Use `1` for a compact single line, or `3` to put cost on its own line. Cost prices are grouped by Provider; each Provider has a `models` array. Model prices are in USD per 1M tokens and match `model.id` or `model.display_name`; `*` at the end acts as a prefix wildcard. `cacheWrite` prices `cache_creation_input_tokens`, and `cacheRead` prices `cache_read_input_tokens`. When a configured price matches, that model price is used for the request; Claude Code's `cost.total_cost_usd` is only used as a fallback when no configured price matches.

The statusline uses English labels:

```text
❬Sonnet❭ 📁 project 🌿 main 🕒 15:04:05
ctx 72% free ▰▰▰▰▰▰▰▰▰▱▱▱ 56k/200k  ⏳ 4:38  Session $0.012 Today $0.31 Week $1.24 Month $5.80
```

Context color is based on percentage free: `70-100` uses muted cyan-green (`38;5;114`), `40-69` amber (`38;5;215`), `15-39` orange (`38;5;208`), and below `15` soft red (`38;5;203`). The 12-cell context bar uses a battery-style free view: filled cells are remaining context, empty cells are used context. Before Claude Code reports `current_usage`, the statusline shows `100% free` when the context window size is known, or `--% free` when it is not, and hides the cache countdown. When `claude --resume` starts a resumed session in a new Claude process, the statusline may reuse the previous context percentage, but the cache countdown stays hidden until a new assistant reply is completed in the current process. The countdown starts from the last completed assistant `end_turn` transcript event and ignores trailing non-message records such as result summaries. Expired cache is prefixed with a yellow `⚠️`.

When the latest semantic transcript event is `/exit`, the command emits no statusline output. This prevents Claude Code's final `refreshInterval` tick from repainting the statusline after the exit message.

Cache countdown defaults to 300 seconds. Override it when needed:

```bash
CC_LINK_CACHE_TTL=3600 cc-link statusline
```

`statusline enable` merges `~/.claude/settings.json`, preserves other settings, writes `settings.json.bak` before modifying the file, and sets:

```json
{
  "disableAllHooks": false,
  "statusLine": {
    "type": "command",
    "command": "cc-link statusline",
    "refreshInterval": 1
  }
}
```

`disableAllHooks: false` is written because Claude Code disables status line execution when all hooks are disabled.

## Common Flags

```text
--src       set source library root for this run; link stores it in project config
--target    temporarily set target root, default .claude
--config    project config path, default .cc-link/cc-link.json
--dry-run   print actions without changing files
--force     replace an existing symlink that points elsewhere
--prune     apply only: remove stale lock entries not present in config
```
