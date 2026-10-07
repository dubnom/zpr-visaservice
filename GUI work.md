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

### Navigation
- [x] Keep Map as the main view and group Adapters, Actors, Services, Visas, Denials, and DNS under horizontal Status tabs with counts.
- [x] Keep summary status metrics on Map only.
- [x] Add Security, Trusted Sources, Logging, and Editor navigation entries with separators.
- [x] Add a ZPR Config menu item for managing versioned ZPLC configuration drafts. Validate and save drafts without applying them to the live runtime.
- [x] Add Trusted Sources links to provider control GUIs using server-configured provider URLs; hide links for providers without a configured manager.
- [x] Rename Logging to "Adapter Logs" and have it show the adapter logs. Remove the arrow. Log Manager remains a separate Tools link.
- [x] When the visa count is zero, show the same circle, but just as an outline with nothing in it.
- [x] When the user interacts through panning and zooming, turn off Auto-fit.

### Service logs and statistics
- [x] Add one Control Room diagnostics view for logs and current stats from every ZPR node and every configured trusted/required service used by Visa Service.
- [x] Standardize ingestion on OpenTelemetry through a ZPR observability trusted service. Put a provider-neutral catalog/query contract behind Control-Service; OpenObserve is the initial replaceable backend, not a UI/API dependency.
- [x] Show source identity/type, last update, stale/unavailable states, bounded searchable logs, and service stats. Keep credentials and provider queries server-side; do not depend on Simulator.
- [x] Test nodes and each configured service class, unavailable/stale sources, redaction/size limits, and Control Room behavior with Simulator unavailable.

### Security review
- [x] Remove the investigation-leads read-only heading and redundant baseline/dismissal explanations.
- [x] Label the browser-local inventory as Baseline and align its count and timestamp with Current.
- [x] Remove the redundant Security Review heading and place Reset Baseline beside the baseline summary.
- [x] Remove the Scan complete message.
- [x] Use explicitly labeled compact controls: Show dismissed controls visibility; Select visible is the bulk-selection checkbox.
- [x] Highlight the nav when a high alert is noticed. This may entail changes to the prioritization of detections (for example - Actor first observed, is not a high alert; multiple requests being blocked could be a high alert - especially if addresses aren't found, varied attempts, etc.)
- [x] Clear the security highlight in the nav once the page is visited (until another security alert)
- [x] Make the security highlight only color the little side indicator, not the main body area.

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
- [x] Still things to do to get the look and feel better. "Validate syntax" should act like "Analyze". There should be a gutter for errors. A Format button, a file button.
- [x] Where are the colors? Where is the gutter? Why is the "Save Draft" not part of the file pulldown?

### Trusted Sources
- [x] Get rid of "Read Only"
- [x] Add a typical LDAP tree view.
- [x] Don't auto refresh ldap.


## Simulator

### Agents
- [x] Remove the machine count label.
- [x] Change Simulated Fleet to Devices and make the heading prominent.
- [x] Change the type filter's All Machines option to All Devices.

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

### Activity
- [x] Remove the Decisions and Blocked flows headings.
- [x] Show visas and denials in sortable tables.
- [x] Place Refresh beside the Stream status in the banner.

### Organizations
- [x] Move Activate/Active control next to the organization name in the identity/policy/services pane.
- [x] Show a shortform list of scenarios in the organization identity area; links open scenarios without activating the organization.