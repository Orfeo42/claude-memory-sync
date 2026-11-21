In addition to condensing, also judge STALENESS.

For entries that are no longer useful, emit `===DELETE: <name>.md===` for them, using the same output contract described above:

- Resolved issue-status write-ups (a bug or incident entry whose fix has since shipped and is no longer at risk of recurring).
- Completed migration or process history that has finished and left no ongoing gotcha (the migration itself, not any durable lesson learned from it).
- Superseded decisions — an entry that documents a decision a later entry explicitly reverses or replaces (delete the superseded one, keep the current one).

Do NOT delete on age alone. The following are never stale just because they are old:

- Durable gotchas and root-cause knowledge (the kind of thing that would bite someone again if forgotten).
- Reference material (facts about how a system/API/tool behaves that remain true).
- User preferences and standing conventions.

When unsure whether an entry is stale, keep it — do not emit a `===DELETE:` for it.

This staleness judgment is appended to, and shares the output contract with, the condense pass above: `===FILE:`/`===DELETE:`/`NOTHING`, nothing else.
