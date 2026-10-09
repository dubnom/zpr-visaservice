# GUI design guidelines

[Active backlog](GUI%20work.md) | [Completed work](GUI%20completed%20work.md)

These are durable presentation rules, not a release checklist. Original wording
is retained below. For implemented behavior and later refinements, consult the
completed-work archive; current open requests are in the active backlog.

## Design Guidelines
- Values that are refreshed automatically should pulse (in a consistent way).
- Extra text, descriptions, etc. are frowned upon.
- If something needs clarity, put it in the help text.
- Data should generally be shown in a standardize sortable table. This isn't necessary for small data sets.
- Numeric table headers should right justify.
- Numbers in tables should right justify.
- Table headers should be bold.
- The topmost banner is only for application-wide information and control.
- No font sizes less than 11px - this doesn't apply to the maps.
- Window Maximize/Restore controls use consistent icons across apps: a single outlined window for Maximize and overlapping windows for Restore. Keep action-specific accessible names, tooltips and pressed state; do not apply these icons to restoring archived records or base state.
