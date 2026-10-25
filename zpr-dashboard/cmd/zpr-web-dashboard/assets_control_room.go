package main

import "embed"

// This explicit production bundle is checked against packaging/control-room/assets.txt.
// New files in static must not become production assets implicitly.
//
//go:embed static/ansi_up.LICENSE static/ansi_up.js static/app.css static/app.js
//go:embed static/assertions.css static/assertions.js static/assistant-core.js static/budget-authority.html
//go:embed static/budget-authority.js static/change-review.css static/change-review.html static/change-review.js
//go:embed static/code-editor.css static/code-editor.js static/colored-log.js static/config-editor.js
//go:embed static/config-form.js static/control-room-typography.css static/diagnostics.css static/diagnostics.js
//go:embed static/editor-assistant.css static/editor-assistant.js static/editor-page.js static/editor-search.js
//go:embed static/gateways.css static/gateways.js static/geography-land.svg static/geography.css
//go:embed static/geography.js static/help.css static/index.html static/log-format.js
//go:embed static/log-panel.js static/machine-logs.css static/machine-logs.js static/map-traffic.js
//go:embed static/node-logs.js static/node-stats.css static/node-stats.js static/operator-login.js
//go:embed static/operator-request.js static/page-runtime.js static/policy-browser.css static/policy-browser.html
//go:embed static/policy-browser.js static/policy-record-icons.js static/polling-display.js static/provisioning-contract.js
//go:embed static/provisioning-registry.js static/provisioning.css static/provisioning.js static/safe-display.js
//go:embed static/security-review.css static/security-review.js static/sidebar.css static/sidebar.js
//go:embed static/source-metrics.js static/subject-link-publications.html static/subject-link-publications.js static/subject-links.html static/subject-links.js static/table-sort.js static/trusted-source.css static/trusted-source.js
//go:embed static/vendor/CODEMIRROR-LICENSES static/vendor/PLURALIZE-LICENSE static/vendor/code-editor.js static/vendor/pluralize.js
var controlRoomFiles embed.FS
