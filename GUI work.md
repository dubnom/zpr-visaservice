# GUI Improvements

## Overall

### Help
- [x] Put help button in the upper-right corner of the page (when it is in the nav it is unclear that it is page specific).
- [x] Show the help text in a pop-up dialog window
- [x] Improve the help text explanation of what things are and what they do.
- [x] Show examples when applicable (mainly the editors and organization or scenario creation).
- [x] Policy and assertion help text needs much more - ZPL guide, assertion guide, explanation of the security/service/client security model, where trusted sources fit in...
- [x] Move the uptime and refresh to be next to the help button.
- [x] Help text is still not following the checked off Improve directive above.

### Page headers
- [x] Get rid of page title since the nav already indicates it.
- [x] Pages shouldn't have their own "REFRESH" buttons when they are part of the global refresh updates.
- [x] Only one line between the topmost status/refresh panel and the content below.

### All text editors
- [x] Add search and replace functionality. Put the buttons on the right side before History. Shared local controls cover policy/assertion, ZPR Config, scenario JSON, and directory LDIF source editors.
- [x] Add AI Assistant
- [x] Make Search and Replace one button/ Clicking the button should show or hide the dialog. Button should change colors while in use.
- [x] Remove "Enter text..." in Search and Replace.
- [x] Add a checkbox to Search and Replace to enable/disable regular expressions
- [x] Remove line and column numbers from Search and Replace.
- [x] Replace needs to support undo in the editor.
- [x] Rename Search & Replace to Find & Replace
- [x] Find & Replace button doesn't change color.
- [x] Make the Find & Replace dialog mimic Microsoft Word but with support for regular expressions (if checked).
- [x] Word wrap should be a check box, and default to enabled
- [ ] Analyze and Format should be next to File...
- [ ] Analyze should use the same colors, fonts, etc. as the Policy editor.
- [ ] Change "Word wrap" to "Wrap".

### All log viewers
- [ ] Word wrap checkbox and default to enabled.
- [ ] Change "Word wrap" to "Wrap"

## Control Room

### Map
- [x] Animate retained adapters when topology bounds move, avoiding sudden jumps.
- [x] Fit centers the rendered topology bounds with padding and preserves glyph proportions.
- [x] Remove the large empty area below the Map canvas.
- [x] Adapter connections to nodes sometimes cover connections between nodes. This may be a generic problem with the map placement algorhythm.
- [x] Auto-fit (fit after/during refresh) should be a checkbox.
- [x] Use solid lines for connecting components.
- [x] Put a little number beside adapters and services showing how many active visas they have.
- [x] Make pulsing even more apparant.
- [x] not all of the connectors are starting at the edges (non circulat shapes show this issue more than others).
- [x] add a dark mode check box.
- [x] Since Nodes don't show visa count - show the count of buffered denies in a similar fashion but use red (and the same outline rules)
- [x] When the number of visas or buffered denies change, do a little pulse.
- [x] Pulse the connector in addition to the adapter.
- [x] Does the map come up blank because it is missing info, or just waiting for a refresh to fill it?
- [x] Possibly related - when I switch to the map from another page, the map is only partially drawn until a refresh.
- [x] Show management and per-fastpath packet-processing counters in node details when a node is clicked, with explicit unavailable/stale telemetry states.
- [x] Add a right mouse click on adapters to highlight current visa allowed services and routes. Right click again on the adapter, or in blank space to remove the highlight.
- [x] Hide the dark mode checkbox for now.
- [x] The visa and buffered denies still don't pulse as far as I can tell. Are we using our expand/contract paradigm.
- [x] Make the right-click highlight stuff work for services as well.
- [x] Right-click on visa count should show the visas.
- [x] Right-click on Buffer denials should show the details.
- [x] Move the small number indicator to overlap the top right corner of the objects. For the visa service put it overlapping the center of the upper right line. For circles upper right line and overlap it. For future shapes do the same kind of thing.
- [x] For 0 visa or buffered denials, fill in the circle with white.
- [x] When the visa count is zero, show the same circle, but just as an outline with nothing in it.
- [x] When the user interacts through panning and zooming, turn off Auto-fit.
- [x] Make right-click on a component should have the same behavior as right-clicking on its number.
- [x] Clicking on the component legend items should highlight what has been clicked on.
- [x] No obvious way to clear highlighted areas or dimmed components.(or the component) Closing the info dialog on the right doesn't always remove the highlight.
- [x] This gets worse with the click-on legend.
- [ ] The right-click (and legend) behavior should simply highlight, no dimming of the other stuff.

### Navigation
- [x] Keep Map as the main view and group Adapters, Actors, Services, Visas, Denials, and DNS under horizontal Status tabs with counts.
- [x] Keep summary status metrics on Map only.
- [x] Add Security, Trusted Sources, Logging, and Editor navigation entries with separators.
- [x] Add a ZPR Config menu item for managing versioned ZPLC configuration drafts. Validate and save drafts without applying them to the live runtime.
- [x] Add Trusted Sources links to provider control GUIs using server-configured provider URLs; hide links for providers without a configured manager.
- [x] Rename Logging to "Adapter Logs" and have it show the adapter logs. Remove the arrow. Log Manager remains a separate Tools link.
- [x] Move "Gateways" and "ZPR Config" under editor. Rename editor 'Policy".
- [x] If there is a subtle way of adding a title to the policy/gateways/... group, call it 'Configuration'.
- [x] Move Log Manager to below Adapter Logs. Make the arrow more visible.

### Service logs and statistics
- [x] Add one Control Room diagnostics view for logs and current stats from every ZPR node and every configured trusted/required service used by Visa Service.
- [x] Standardize ingestion on OpenTelemetry through a ZPR observability trusted service. Put a provider-neutral catalog/query contract behind Control-Service; OpenObserve is the initial replaceable backend, not a UI/API dependency.
- [x] Show source identity/type, last update, stale/unavailable states, bounded searchable logs, and service stats. Keep credentials and provider queries server-side; do not depend on Simulator.
- [x] Test nodes and each configured service class, unavailable/stale sources, redaction/size limits, and Control Room behavior with Simulator unavailable.
- [x] Put "Monitoring" as a header for the Map, Status... area.
- [x] Add another separator below ZPR Config named "Provisioning"
- [x] Add the new provisioner to the nav and call it "Adapters"

### Security review
- [x] Remove the investigation-leads read-only heading and redundant baseline/dismissal explanations.
- [x] Label the browser-local inventory as Baseline and align its count and timestamp with Current.
- [x] Remove the redundant Security Review heading and place Reset Baseline beside the baseline summary.
- [x] Remove the Scan complete message.
- [x] Use explicitly labeled compact controls: Show dismissed controls visibility; Select visible is the bulk-selection checkbox.
- [x] Highlight the nav when a high alert is noticed. This may entail changes to the prioritization of detections (for example - Actor first observed, is not a high alert; multiple requests being blocked could be a high alert - especially if addresses aren't found, varied attempts, etc.)
- [x] Clear the security highlight in the nav once the page is visited (until another security alert)
- [x] Make the security highlight only color the little side indicator, not the main body area.
- [x] Flag sustained aggregate DNS NXDOMAIN probing in Security Review as an unattributed review finding; do not raise the high-priority nav alert from server-wide counters.

### Editor
- [x] If there is a horizontal scrollbar, color the forbidden area under the gutter white.
- [x] Keep every source-located Analyze warning in the gutter on its reported line; never render a duplicate warning list above or below the editor. Keep line-less analysis failures in the status area rather than inventing line 1.
- [x] Keep line-specific policy warnings in the gutter; show analysis failures without a source line in the status area instead of attaching them to line 1.
- [x] Rename "Rescan LDAP" to "Refresh Attributes", and make sure it handles all trusted attribute sources.
- [x] Put "Policy" or "Assertion" above the editor to be clear of the mode. Center between the left and right button groups.
- [x] Get rid of the extra line above the name of the policy/assertion being edited.
- [x] Add an indicator to the policy/assertion name line to indicate if the file has been modified. Don't show anything if it isn't.

### Status
- [x] I noticed on the "adapters" page that even when sorted, the records moved around (though nothing changed). I think sorting only sorts on its primary field but if that field has duplicates this occurs. A solution would be to always have a secondary field (or more) that is/are automatically attached. The user has nothing to do with this but the results would be deterministic.
- [x] Status tables need vertical scroll bars when they get big.

### Adapter Logs
- [x] move the buttons to be next to "Adapter Logs".
- [x] Add a new button toggle for showing and hiding all adapters.
- [x] The Adapter picker should be a dropdown next to the adapter title. That should remove its line.
- [x] A page-wide setting for wordwrap toggling should be added next to the page buttons.
- [x] Add a button to toggle seeing only running adapters.
- [x] The adapter picker dropdown should be a pop-up dialog controlled by a place to click.
- [x] I'm confused by the new Adapter titles.
- [x] Running Only checkbox doesn't work - it shows non-running adapters.
- [x] Remove "Adapter logs" and "Controller logs" from the adapter titles.
- [x] Show the adapter name where you used to show "Adapter logs"... in the same font.
- [x] Make a chevron button next to the adapter name which should display the dropdown/dialog.
- [x] Start with Word wrap enabled.
- [x] The adapter picker doesn't need a header or anything else. It just needs the pull down to start in an open position.
- [x] If there aren't any adapters (none available) than show a message that says this/
- [x] Move the picker to be under the adapter title when opened.

### ZPR Config
- [x] Adopt the same look-and-feel as the policy editor. If ZPR config is limited to one file, there is no need for Browse, or some of the File commands. Refresh Attributes is also not relevant.
- [x] Keep ZPR Config toolbar roles aligned with the Policy editor: File on the left, editor mode centered, Analyze/Format in the right-side action group, and History/utilities at the right. Match the orange pending-analysis state, enabled/disabled Format styling, success/error colors, and responsive placement.
- [x] Still things to do to get the look and feel better. "Validate syntax" should act like "Analyze". There should be a gutter for errors. A Format button, a file button.
- [x] Where are the colors? Where is the gutter? Why is the "Save Draft" not part of the file pulldown? Make "File..." and Analyze use the same color and sizes of the policy editor.
- [x] Remove "New draft" at the bottom of the editor, and use that space for the editor.
- [x] Remove line numbers from the gutter.
- [x] Gutter looks different from the other editors.

### Trusted Sources
- [x] Get rid of "Read Only"
- [x] Add a typical LDAP tree view.
- [x] Don't auto refresh ldap.

### Gateways
- [x] Add a Control Room page listing installed ZPL Gateway services by organization and editing their versioned runtime drafts.
- [x] Validate destination/path/method/timeout/response-size settings through Control-Service before saving; keep drafts organization-scoped and do not activate them.
- [ ] Add verified external-network policy metadata, gateway runtime health, and explicit reviewed activation/rollback after the production provisioning contract is implemented.
- [x] There can be multiple gateways, so lets use the exact same editor paradigm, colors, and behavior as the polic editor.


## Simulator

### Navigation
- [x] Set order to: Organizations, Scenarios, Trusted Sources, Activity, Workers (merged Agents/Workload logs).

### Agents
- [x] Remove the machine count label.
- [x] Change Simulated Fleet to Devices and make the heading prominent.
- [x] Change the type filter's All Machines option to All Devices.
- [x] Merge Agents and Workload Logs as Workers, retaining passive device/runtime inspection and removing active start/stop/login controls (approved).

### Scenarios
- [x] Remove Available scenarios and the catalog-count label.
- [x] Place New Scenario above the list and label it New Scenario....
- [x] Remove the EXECUTION PLAN label from the scenario editor.
- [x] Remove the Unfiled count/header; unfiled scenarios appear without a separate count.
- [x] Show the scenario name and run state together in the run heading.
- [x] Saturate the running state; show Cancel only while running and Clear only after a terminal run.
- [x] Label the published scenario action "Run".
- [x] Pulse the running state only when scenario progress changes; respect reduced-motion preferences.
- [x] Remove the redundant Organizations list heading and profile-count text.
- [x] Keep the Activate/Active control beside the organization name and place its description below.
- [x] Make the scenario running state more visible with more saturated colors. The
Running" orange should be a non-indicated color like blue.
- [x] When a scenario fails, the step/total steps doesn't show the failure line because of the cleanup.
- [x] We need a raw version of a scenario editor (that gives the same type of experience, colors, columns, etc. as the Control Room editors)
- [x] Add AI Assistants to the dialog and raw scenario editors.
- [x] Move the raw/dialog link/button to the top right corner of the inner pages.
- [x] Implement the Analyze button with colors and behavior just like every other editor.
- [x] Switch (or just edit) scenarios in yaml.

### Activity
- [x] Remove the Decisions and Blocked flows headings.
- [x] Show visas and denials in sortable tables.
- [x] Place Refresh beside the Stream status in the banner.

### Organizations
- [x] Move Activate/Active control next to the organization name in the identity/policy/services pane.
- [x] Show a shortform list of scenarios in the organization identity area; links open scenarios without activating the organization.
- [ ] I want to be able to add new organizations with the help of the AI Assistant. This is a bigger project, so make a plan first and get my approval.

### Workers
- [x] Add the wordwrap button like the Adapter Logs.

## Provisioning

Machine/adapter enrollment is tracked in [Provisioning plan](../Provisioning%20plan.md).

- [x] Add a Control Room invitation worksheet, local review, delivery/expiry
  explanations, and enrollment help under Provisioning > Adapters.
- [x] Keep the worksheet memory-only, with explicit unsaved/unvalidated
  labels and no invitation creation, email, or Simulator calls.
- [ ] Connect approved catalogs and invitation/review actions only after HTTPS
  named-user sign-in and independently verified Control-Service delegation.
- [x] Add opt-in direct HTTPS OIDC sign-in, named subject/scope display, explicit
  unavailable/expired states, and CSRF logout without unlocking enrollment.
- [x] Add opt-in signed named-user delegation to the private enrollment API,
  independent backend grants, persistent replay protection, and named audit.
- [x] Load authorized catalogs and organization-scoped paginated registry
  list/fresh details without enabling mutations. Clear on navigation/session loss,
  discard stale responses, and distinguish unavailable from empty.
- [x] Verify read-only registry access through actual HTTPS OIDC login, mTLS
  delegation, SQLite, and logout in desktop/tablet browsers with Simulator unavailable.
- [ ] Wire worksheet selections to approved catalogs, invitation creation/code
  handling, and revision/key-bound review with real-service mutation browser tests.
