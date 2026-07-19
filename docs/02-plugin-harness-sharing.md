# Phase 2 — plugin/harness sharing (future, context only)

Not planned yet. This doc exists so a future session can start Phase 2
without re-deriving the following facts.

## Goal (as stated by user)

"This computer should share plugin and harness (some are zsh function, or
other type of functions, other things are related, we should evaluate
step by step)." Broader than memory files: covers Claude Code plugins,
skills, and shell-level helper functions the user has built up.

## Inventory found during Phase 0/1 survey (orfeo-pc, 2026-07-12)

- **Plugins** (`~/.claude/plugins/installed_plugins.json`): `caveman`
  (from marketplace `caveman`, GitHub `JuliusBrussee/caveman`, user-scope,
  actively used — drives the statusline and the always-on caveman mode
  set in `~/.claude/CLAUDE.md`), plus `pyright-lsp`/`gopls-lsp`/
  `clangd-lsp` from marketplace `claude-plugins-official`.
- **Marketplaces**: `settings.json` has `extraKnownMarketplaces` +
  `enabledPlugins` — this is config, not files. Replicating a plugin on a
  new machine is likely just replaying this config block (marketplace
  add + enable), not file-syncing plugin internals. Needs verification
  once this phase is actually executed (check whether plugins install
  content outside what marketplace-add pulls in, e.g. cached plugin code
  under `~/.claude/plugins/` that isn't just metadata).
- **User-authored skills** (`~/.claude/skills/`): `pr-description`,
  `pr-description-from-tag`. No path-dependency, same shape as
  `rules/*.md` — straightforward to add under `global/` in this repo
  alongside `CLAUDE.md`/`rules/`.
- **zsh helper functions**: referenced throughout `~/.claude/rules/*.md`
  but living outside `~/.claude` entirely, under `~/.zshrc.d/` — at least
  `forgejo.zsh` (Forgejo API wrappers), `keycloak.zsh` (Keycloak
  token/API helpers), and OpenStack-related helpers (`oscloud`, per
  `~/.claude/rules/openstack.md`). Full inventory of `~/.zshrc.d/*.zsh`
  not yet taken — first step of executing this phase.
- **`settings.json` vs `settings.local.json`**: `settings.json`'s
  `permissions.allow` is a curated, safe, read-only baseline (Read/Glob/
  Grep/curl/etc) — sync candidate. `settings.local.json` is a
  180-entry, session-accumulated allowlist, clearly machine/session
  specific, not a sync candidate as-is (see docs/04 — it's actually a
  useful _input_ to usage mining, not something to sync verbatim).

## Open questions to resolve before drafting a real plan

- Does `~/.zshrc.d/` get folded into this same staging repo (as e.g.
  `dotfiles/zshrc.d/`, synced the same way as `global/`), or does taking
  on shell dotfiles push this toward adopting a dedicated dotfiles
  manager (chezmoi — checked, not currently installed/used)? Trade-off:
  reusing this repo's existing Syncthing+git plumbing vs. a
  purpose-built tool.
- For plugins: is there ever plugin-local _state_ (not just config) that
  would need syncing, or is config-replay always sufficient? Needs
  checking against how the Claude Code plugin system actually stores
  installed-plugin data, not assumed.
- Priority ordering: which harness pieces does the user actually want
  first — zsh functions (forgejo/keycloak/openstack) are almost certainly
  higher value than plugin config replay, since plugins are one-time setup
  per machine while zsh functions get hand-edited/extended over time.
