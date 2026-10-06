(() => {
  if (window.__zprSidebar) return;
  window.__zprSidebar = true;
  let navigationObserver;
  let observedNavigation;
  let helpReturnFocus;

  const controlHelp = {
    map: {
      title: "Map",
      intro: "Inspect the live ZPR topology and open a node or adapter to review its identity and current grants.",
      steps: ["Use Fit to frame the current topology.", "Select a node or adapter to inspect its address and recent activity."],
      recovery: "If the map is disconnected or empty, open Status and review the Visa Service connection errors.",
      docs: "Control Room guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    connections: {
      title: "Status",
      intro: "Use the status tabs to inspect actors, services, visas, denials, and DNS data returned by Visa Service.",
      steps: ["Choose a tab to compare live records.", "Use table filters and column headings to narrow or sort results."],
      recovery: "A disconnected status identifies the failing Admin API resource in its error details; verify Control-Service and Visa Service availability.",
      docs: "Control Room guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "security-review": {
      title: "Security review",
      intro: "Compare the browser-local Baseline inventory with the current observed inventory; this view does not replace authoritative policy review.",
      steps: ["Select visible findings to inspect or dismiss them.", "Reset Baseline only when you intend to replace the saved browser snapshot."],
      recovery: "If an inventory is unavailable, refresh the source view and inspect its displayed error before resetting the baseline.",
      context: "A missing or stale source is missing evidence, not proof of a healthy state. Baseline is a local comparison aid; ZPL policy and Visa Service remain authoritative for access decisions.",
      docs: "Security and operations guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    diagnostics: {
      title: "Diagnostics",
      intro: "Review bounded logs and metrics for configured ZPR nodes and trusted or required services, including each source’s freshness and availability.",
      steps: ["Filter by source or search the returned log content.", "Treat stale and unavailable sources as missing evidence, not healthy sources."],
      recovery: "Check the source map, provider credentials, and configured OpenTelemetry streams on Control-Service; provider details remain server-side.",
      context: "The production Control Room uses operator-configured Visa Service and trusted-service telemetry. It does not read Simulator workloads or depend on Simulator availability.",
      docs: "Diagnostics configuration example", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/diagnostics/source-map.example.json",
    },
    sources: {
      title: "Trusted sources",
      intro: "Inspect configured attribute providers and their latest health and lookup status.",
      steps: ["Open a provider link only when you need its operator console.", "Use the assertion editor’s Read source action to inspect returned directory data."],
      recovery: "For unavailable sources, check provider health, identity mappings, and the Control-Service trust configuration.",
      context: "Providers are configured and authenticated server-side. The browser uses same-origin Control Room APIs and does not receive provider URLs, tokens, or directory credentials. Trusted attributes inform identity and policy; the provider does not grant access.",
      docs: "Trusted-service configuration", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/README.md",
    },
    "adapter-logs": {
      title: "Adapter logs",
      intro: "Read logs from the operator-configured adapter and controller inventory. Sources are bounded and read-only.",
      steps: ["Choose an adapter and log source.", "Use search and follow controls to inspect recent entries."],
      recovery: "An unavailable source is reported independently; check its configured path/container and the Control-Service log inventory.",
      docs: "Adapter log configuration", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "zpr-config": {
      title: "ZPR configuration",
      intro: "Edit and validate versioned ZPLC drafts. Saving a draft does not apply it to the live runtime.",
      steps: ["Choose a configuration to edit, then Analyze and Format as needed.", "Save a validated draft; use the explicit deployment workflow to apply a candidate."],
      recovery: "Compiler diagnostics are attached to source lines when available. Resolve them before staging; a staged candidate is not an activated policy.",
      docs: "ZPLC and configuration guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/README.md",
    },
    policy: {
      title: "Policy editor",
      intro: "Browse versioned policy records, edit source, and use Analyze to compile and test against the configured candidate fixtures.",
      steps: ["Analyze reports compiler and evaluator results in the source gutter.", "Format changes spacing only; it does not save.", "Stage creates a candidate for review; it does not deploy or activate policy."],
      recovery: "Analysis unavailable means required test fixtures could not be built; it is distinct from a compiler error. Check the status message and fixture-provider configuration.",
      context: "ZPL matches authenticated client/user or device attributes and service identity attributes. Service rules describe a destination; `allow` grants matching client classes access. The browser calls same-origin `/api/*` and carries no Visa Service or Policy Repository credentials.",
      example: "define FinanceStaff as a user with user.department:Finance;\ndefine FinanceWorkspace as a service with device.zpr.adapter.cn:finance-workspace;\nservice FinanceWorkspace as json {\"service_class\":\"FinanceWorkspace\",\"actor_cn\":\"finance-workspace\",\"summary\":\"Illustrative finance reporting and ledger workspace.\",\"endpoint\":\"zpr://finance-workspace\",\"status\":\"example only; not deployed\"}.\n  allow FinanceStaff.",
      docs: "Policy authoring guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/README.md#control-room-and-policy-service",
    },
    assertions: {
      title: "Assertion editor",
      intro: "Author report-only checks against configured trusted sources. Analyze evaluates the current source; it does not save it.",
      steps: ["Use Analyze to see pass/fail results and source-line warnings in the gutter.", "Format normalizes layout without saving.", "Save stores the assertion revision; periodic evaluation is a separate setting."],
      recovery: "If trusted data cannot be read, verify the provider and its organization-scoped identity mapping before interpreting assertion results.",
      context: "Assertions validate trusted directory data and never grant or deny network access. Trusted readers and credentials are configured on the server; the browser cannot submit provider URLs, credentials, or data snapshots. Lint warnings are advisory.",
      example: "// Assertions are report-only data checks.\ngroup \"Operators\" members >= 2;\npeople in \"Operators\" attribute \"mail\" present;",
      docs: "Assertion language and behavior", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/ASSERTIONS.md",
    },
  };

  const simulatorHelp = {
    "agents.html": {
      title: "Devices",
      intro: "Inspect simulated devices, runtime services, and recent agent/controller activity.",
      steps: ["Open a device to review its state.", "Log users out and stop active sessions before changing organizations."],
      recovery: "If a controller is offline, verify the machine-control listener and device container state before retrying a command.",
      context: "A simulated client workload and a service workload have distinct ZPR identities. A test request succeeds only when the active runtime policy grants that client access to the service; starting a workload does not create a grant.",
      docs: "Simulator guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "organizations.html": {
      title: "Organizations",
      intro: "Each organization selects a separate simulated policy, directory, DNS context, and runtime profile.",
      steps: ["Review the target profile before activation.", "Switching resets the simulated runtime and can interrupt connections and workloads; finish/cancel scenarios and log out users first.", "Use Open reset log after failures to inspect the exact startup gate."],
      recovery: "A failed activation rolls back the selected organization. Check the reset log, then verify policy, assertion source, LDAP, DNS, and Control-Service before retrying.",
      context: "Example: Great Lakes Instruments supplies its own directory base, identity mappings, services, and role checks. Activation changes Simulator context and may reset its workloads; it does not deploy network policy or reseed LDAP.",
      docs: "Organization runtime profiles", href: "https://github.com/org-zpr/zpr-visaservice/tree/main/zpr-dashboard/cmd/zpr-web-dashboard/examples/organizations",
    },
    "scenarios.html": {
      title: "Scenarios",
      intro: "Scenarios are organization-specific, revisioned workflows for simulated runtime tasks.",
      steps: ["Only published revisions can run.", "Review cleanup steps before publishing; cancel stops active work and still runs cleanup.", "Editing or publishing does not activate a different organization."],
      recovery: "If the catalog is empty, confirm the active organization has bundled or workspace scenarios. Open the reset log for activation failures.",
      context: "Example: the Northstar `client-service` scenario runs `finance-client` on machine-03 against `echo-service` on machine-05. It requires the separately provisioned EchoWeb TCP 8080 grant; scenario execution does not modify policy.",
      example: "{\"id\":\"first-request\",\"after\":[\"echo-ready\"],\"action\":\"request_test_service\",\"machine\":\"machine-03\",\"component\":\"finance-client\",\"target\":\"echo-service\"}",
      docs: "Scenario format and actions", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/examples/scenarios/README.md",
    },
    "activity.html": {
      title: "Activity",
      intro: "Review recent Visa Service grants and denials observed in the active simulated runtime.",
      steps: ["Sort columns to compare flows and outcomes.", "The view polls automatically while this page is active."],
      recovery: "An empty table can mean no recent activity or an unavailable Admin API; inspect the connection status before drawing conclusions.",
      context: "This is a periodically refreshed Simulator view of Visa Service activity. A grant or denial is an observed runtime decision, not a policy-editor preview.",
      docs: "Simulator guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "machine-logs.html": {
      title: "Workload logs",
      intro: "Inspect bounded controller, adapter, and workload output from the simulator fleet.",
      steps: ["Choose the device and source to read.", "Pause or filter the stream when examining a specific event."],
      recovery: "Unavailable sources are reported separately; confirm the container is running and the requested log source exists.",
      context: "The Simulator polls the selected workload feed while this page is active. These application logs are separate from the production Control Room’s operator-configured adapter, controller, and service telemetry.",
      docs: "Simulator guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "trusted-source.html": {
      title: "Trusted source",
      intro: "Inspect the active organization’s read-only trusted directory attributes and source health.",
      steps: ["Refresh the selected source after an organization activation.", "Use the displayed identity and base DN to confirm you are inspecting the intended directory."],
      recovery: "Check the activation log and the organization’s LDAP bind/service health if the source cannot be read.",
      context: "This read-only view displays attributes returned for the active Simulator organization. It neither configures the trusted reader nor grants access; policy evaluates authenticated identities and approved attributes separately.",
      docs: "Trusted-source profile fields", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
  };

  const storageKey = () => document.querySelector(".sidebar .brand")?.textContent.includes("SIMULATOR")
    ? "zpr.simulator.sidebar-condensed" : "zpr.control-room.sidebar-condensed";

  function helpContent() {
    const simulator = document.querySelector(".sidebar .brand")?.textContent.includes("SIMULATOR");
    if (simulator) return simulatorHelp[location.pathname.split("/").pop()] || simulatorHelp["agents.html"];
    const page = location.hash.slice(1) || "map";
    if (page === "policy" && !document.querySelector("#policy-assertion-editor")?.hidden) return controlHelp.assertions;
    return controlHelp[page] || controlHelp.map;
  }

  function updateHelp() {
    const dialog = document.querySelector("#zpr-help-dialog");
    if (!dialog) return;
    const content = helpContent();
    dialog.querySelector("#zpr-help-title").textContent = content.title;
    dialog.querySelector("#zpr-help-intro").textContent = content.intro;
    dialog.querySelector("#zpr-help-steps").replaceChildren(...content.steps.map((text) => {
      const item = document.createElement("li");
      item.textContent = text;
      return item;
    }));
    dialog.querySelector("#zpr-help-recovery").textContent = content.recovery;
    const contextSection = dialog.querySelector("#zpr-help-context-section");
    contextSection.hidden = !content.context;
    dialog.querySelector("#zpr-help-context").textContent = content.context || "";
    const exampleSection = dialog.querySelector("#zpr-help-example-section");
    exampleSection.hidden = !content.example;
    dialog.querySelector("#zpr-help-example").textContent = content.example || "";
    const docs = dialog.querySelector("#zpr-help-docs");
    docs.textContent = content.docs;
    docs.href = content.href;
  }

  function ensureHelp() {
    if (!document.querySelector("link[data-zpr-help-styles]")) {
      const stylesheet = document.createElement("link");
      stylesheet.rel = "stylesheet";
      stylesheet.href = "/help.css?v=2";
      stylesheet.dataset.zprHelpStyles = "true";
      document.head.append(stylesheet);
    }
    const topbar = document.querySelector(".main-content .topbar, main .topbar, .topbar");
    let button = document.querySelector(".help-trigger");
    if (!button) {
      button = document.createElement("button");
      button.type = "button";
      button.className = "quiet help-trigger";
      button.textContent = "Help";
      button.setAttribute("aria-label", "Help for this page");
      button.setAttribute("aria-haspopup", "dialog");
      button.setAttribute("aria-controls", "zpr-help-dialog");
      button.title = "Help for this page";
    }
    if (topbar && button.parentElement !== topbar) topbar.append(button);
    if (!document.querySelector("#zpr-help-dialog")) {
      const dialog = document.createElement("dialog");
      dialog.id = "zpr-help-dialog";
      dialog.className = "zpr-help-dialog";
      dialog.setAttribute("aria-labelledby", "zpr-help-title");
      dialog.setAttribute("aria-describedby", "zpr-help-intro");
      dialog.innerHTML = `<header><div><p class="eyebrow">HELP · CURRENT PAGE</p><h2 id="zpr-help-title"></h2></div><button class="quiet" type="button" data-help-close aria-label="Close help">×</button></header><p id="zpr-help-intro"></p><section><h3>Workflow</h3><ol id="zpr-help-steps"></ol></section><section id="zpr-help-context-section" hidden><h3>Identity and trust model</h3><p id="zpr-help-context"></p></section><section id="zpr-help-example-section" hidden><h3>Example</h3><pre><code id="zpr-help-example"></code></pre></section><section><h3>If something fails</h3><p id="zpr-help-recovery"></p></section><footer><a id="zpr-help-docs" target="_blank" rel="noopener noreferrer"></a><button class="quiet" type="button" data-help-close>Close</button></footer>`;
      document.body.append(dialog);
      dialog.querySelectorAll("[data-help-close]").forEach((button) => button.addEventListener("click", () => dialog.close()));
      dialog.addEventListener("close", () => {
        if (helpReturnFocus?.isConnected) helpReturnFocus.focus();
      });
    }
  }

  function condensed() {
    try { return localStorage.getItem(storageKey()) === "true"; }
    catch { return document.body.classList.contains("sidebar-condensed"); }
  }

  function updateNavigationHints() {
    const sidebar = document.querySelector(".sidebar");
    if (!sidebar) return;
    for (const link of sidebar.querySelectorAll(".nav-link")) {
      if (!link.title) link.title = link.textContent.trim();
    }
  }

  function applyState(collapsed) {
    document.body.classList.toggle("sidebar-condensed", collapsed);
    const button = document.querySelector(".sidebar-toggle");
    if (!button) return;
    const name = collapsed ? "Expand side menu" : "Condense side menu";
    button.setAttribute("aria-expanded", String(!collapsed));
    button.setAttribute("aria-label", name);
    button.title = name;
    button.querySelector("span").textContent = collapsed ? "\u203a" : "\u2039";
    updateNavigationHints();
  }

  function initialize() {
    const sidebar = document.querySelector(".sidebar");
    if (!sidebar) return;
    if (!sidebar.querySelector(".sidebar-toggle")) {
      sidebar.id ||= "app-sidebar";
      const header = document.createElement("div");
      header.className = "sidebar-header";
      const button = document.createElement("button");
      button.type = "button";
      button.className = "sidebar-toggle";
      button.setAttribute("aria-controls", sidebar.id);
      const icon = document.createElement("span");
      icon.setAttribute("aria-hidden", "true");
      button.append(icon);
      header.append(button);
      sidebar.prepend(header);
    }
    const brand = sidebar.querySelector(".brand");
    const header = sidebar.querySelector(".sidebar-header");
    if (brand) {
      sidebar.classList.add("sidebar-brand-header");
      if (header && brand.parentElement !== header) header.prepend(brand);
    }
    const navigation = sidebar.querySelector(".primary-nav");
    if (navigation) ensureHelp();
    if (navigation && observedNavigation !== navigation) {
      navigationObserver?.disconnect();
      observedNavigation = navigation;
      navigationObserver = new MutationObserver(updateNavigationHints);
      navigationObserver.observe(navigation, { childList: true, subtree: true, attributes: true, attributeFilter: ["class", "aria-current"] });
    }
    applyState(condensed());
  }

  document.addEventListener("click", (event) => {
    const help = event.target.closest(".help-trigger");
    if (help) {
      ensureHelp();
      updateHelp();
      helpReturnFocus = help;
      const dialog = document.querySelector("#zpr-help-dialog");
      if (!dialog.open) dialog.showModal();
      dialog.querySelector("[data-help-close]").focus();
      return;
    }
    if (!event.target.closest(".sidebar-toggle")) return;
    const collapsed = !document.body.classList.contains("sidebar-condensed");
    try { localStorage.setItem(storageKey(), String(collapsed)); } catch {}
    applyState(collapsed);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    const dialog = document.querySelector("#zpr-help-dialog");
    if (!dialog?.open) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    dialog.close();
  }, true);
  window.addEventListener("storage", (event) => { if (event.key === storageKey()) initialize(); });
  window.addEventListener("hashchange", () => { if (document.querySelector("#zpr-help-dialog")?.open) updateHelp(); });
  window.addEventListener("policy-record-kind-changed", () => { if (document.querySelector("#zpr-help-dialog")?.open) updateHelp(); });
  document.addEventListener("simulator:activate", initialize);
  const bodyObserver = new MutationObserver(initialize);
  bodyObserver.observe(document.body, { childList: true });
  initialize();
})();