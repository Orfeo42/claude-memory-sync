# Feature request draft — inject theme tokens into full-code app iframes

Target: https://github.com/windmill-labs/windmill/issues (not opened yet).

---

Title: Expose the instance theme's `--color-*` tokens to full-code (raw) appsa

## Problem

Full-code apps (React/Svelte raw apps) render in an iframe, so the host
UI's stylesheet and CSS custom properties do not reach them. The only
theme signal the preview shell currently forwards is a `setDarkMode`
message that sets `color-scheme` and toggles a `windmill-dark` class on
the iframe's root element.

That tells an app _that_ the theme is dark, but not _what the theme looks
like_. Windmill's own UI is fully tokenized — every color is consumed as
`rgb(var(--color-<token>))`, with token sets per theme (light, dark,
custom themes such as the GitHub-dark variant). None of those values are
available inside the app iframe, so every full-code app that wants to
match the surrounding UI has to hardcode a copy of the token values and
manually keep them in sync with upstream palette changes.

Workaround we use today: because the preview/app iframe is same-origin
and unsandboxed, the app reads
`getComputedStyle(window.parent.document.documentElement)
.getPropertyValue('--color-…')` for each token at startup, mirrors the
values onto its own root, and watches the parent `<html>` with a
MutationObserver to follow theme switches. It works, but it depends on
same-origin + no-sandbox internals and breaks the moment an app runs
sandboxed or the DOM/token internals change.

## Proposal

Forward the resolved theme tokens to the app iframe as part of the
existing theme plumbing, so app CSS can use the same
`rgb(var(--color-<token>))` expressions as the host UI. Either:

1. Extend the existing `setDarkMode` message to include the resolved
   token map (`{ dark: boolean, tokens: Record<string, string> }`), and
   have the preview shell (`app-preview.html`) set each entry as
   `--color-<token>` on `document.documentElement`; or
2. Have the shell inject a `<style>:root { --color-…: …; }</style>`
   block it regenerates on theme change.

Either way the contract becomes: inside a full-code app,
`var(--color-surface-primary)`, `var(--color-text-primary)`, etc. are
always defined and always match the host theme, including custom themes,
with no same-origin assumptions.

## Benefits

- Full-code apps can match the surrounding UI without duplicating the
  palette or poking at the parent document.
- Works under a sandboxed iframe policy (the current workaround does
  not).
- Custom instance themes propagate to apps for free.
- The token vocabulary already exists and is stable
  (`frontend/src/lib/assets/tokens/tokens.json`); this only transports
  it across the iframe boundary.
