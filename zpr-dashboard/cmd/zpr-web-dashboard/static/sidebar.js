(() => {
  if (window.__zprSidebar) return;
  window.__zprSidebar = true;
  let navigationObserver;
  let observedNavigation;
  let helpReturnFocus;

  const controlHelp = {
    map: {
      title: "Map",
      intro: "This diagram is the current Visa Service view of the ZPR network. Nodes host adapters; adapters make the ZPR connection on behalf of an endpoint; registered service badges name destinations published by an actor.",
      steps: ["Fit frames all rendered actors and services without distorting the diagram; + and − zoom the view. Auto-fit is checked initially: refreshes fit the graph automatically. Panning or zooming turns it off to retain your chosen view. Recheck it to resume automatic fitting; Fit remains a one-time action without changing the checkbox.", "Search highlights matching actors, addresses and service names without removing surrounding connections.", "Select an actor, registration or link to see its reported identity, address and live details. Solid dock lines attach adapters to nodes; inter-node lines are reported network connections.", "Small numbers beside adapters count active visas involving either endpoint. Service counts match address, protocol and service-side port; services sharing an endpoint share its count. An empty outline means zero; a ? means the complete inventory is unavailable.", "New grants glow green around the requester and matching services; new denials glow red around the requester. Dock wires thicken in the same color. Reduced-motion mode keeps the color feedback without expanding glyphs."],
      recovery: "Right-click an adapter to highlight services and ordered node routes allowed by its current outbound visas. Right-click it again, or blank canvas, to clear. Missing route data is reported rather than inferred. Dark mode is temporarily hidden. Red node badges show the latest fresh count of expiring buffered denials, not lifetime denials. Empty outlines mean zero; ? means telemetry is unavailable. Click a node for packet-processing counters and local denial occurrences. Changing counts expand and contract twice over 2.4 seconds, including decreases to zero; reduced-motion mode uses fading instead. Service and dock connectors pulse with decisions. Returning to Map draws the latest cached snapshot immediately. Loading topology means the first request is pending; No nodes or adapters means a connected snapshot is empty; Topology unavailable or Unable to load topology means the API could not supply it. Review the connection errors and use Refresh to retry.",
      context: "The map is observed runtime topology, not a policy editor or deployment preview. A visible connection does not by itself mean a client is authorized to access a service; Visa Service grants depend on authenticated identities and the active policy.",
      example: "A client adapter is docked to node A → node A has an inter-node connection to node B → node B hosts an adapter that registers a service. Select each line or actor to inspect what Visa Service actually reported.",
      docs: "Control Room guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    connections: {
      title: "Status",
      intro: "Status tabs are live Visa Service Admin API snapshots. They describe reported runtime state; they do not edit configuration or explain a policy decision beyond the fields shown.",
      steps: ["Choose Adapters, Actors, Services, Visas, Denials or DNS in the horizontal tabs.", "Type in Filter to match visible row values. Select a column heading to sort; ties are ordered by the other row fields for stable results.", "Use the global refresh interval, Pause updates and Refresh controls in the top bar for every Control Room view."],
      recovery: "A disconnected status identifies the failing Admin API resource in its error details; verify Control-Service and Visa Service availability.",
      context: "The tab count is the number of records in the latest successful snapshot. When an API resource is unavailable, the page reports that independently; an empty result is not evidence that the service is healthy.",
    },
    actors: {
      title: "Actors",
      intro: "Actors are identities Visa Service knows about. Node rows describe infrastructure; adapters and clients describe attached or communicating ZPR endpoints.",
      steps: ["Filter by identity, role, address or state.", "Select an actor row to inspect its reported address and available runtime details.", "Use NODE STATE and the node details to distinguish current reachability from identity enrollment."],
      recovery: "Missing actors can reflect Admin API availability, registration or stale topology. Compare with Map and the global connection status before treating absence as a removal.",
      context: "An actor identity alone does not authorize traffic. A policy grant applies to authenticated client identities accessing registered service identities.",
    },
    services: {
      title: "Services",
      intro: "Services are destination endpoints registered by provider actors. The row associates the advertised service name and kind with its registering actor, DNS name and endpoints.",
      steps: ["Filter by service name, kind, provider actor, address or endpoint.", "Use Type colors to identify service kinds; the legend is only a visual key.", "Select a row to inspect the service registration and provider details."],
      recovery: "A registered service may still be unavailable or inaccessible. Check provider actor state and Diagnostics; registration is not an access grant.",
      context: "Clients request a service identity. Visa Service evaluates that authenticated client/service pair against active policy before issuing a visa.",
    },
    visas: {
      title: "Visas",
      intro: "A visa is an observed authorization issued for a network flow. This table shows the visa identifier, endpoint pair, protocol, requesting node and expiry.",
      steps: ["Filter by endpoint, protocol, node or visa ID.", "Sort Expires to find grants nearing expiry.", "Use the endpoint address details to relate a flow back to actors and topology."],
      recovery: "No active visas can mean no matching requests or expired grants. Check Denials and Visa Service status before diagnosing policy.",
      context: "This is runtime evidence of an issued authorization, not a guarantee that the endpoint remains reachable or that a new request will receive the same decision.",
    },
    denies: {
      title: "Denials",
      intro: "Denials summarize recent blocked requests by source, destination, protocol/port and reason, with the observed hit count.",
      steps: ["Sort Hits to find repeated blocked flows.", "Filter by address, reason or port to group related attempts.", "Compare source and destination actors with the Map before changing policy."],
      recovery: "A denial means a request was blocked, but the table alone may not identify why an identity or rule failed. Review the reason and the authoritative policy/test evidence.",
      context: "Repeated or varied denials from an unknown source may be a security signal. Security highlights selected high-priority patterns; it does not automatically label every denied request as hostile.",
    },
    dns: {
      title: "DNS",
      intro: "DNS shows the configured zone’s request counters, zone statistics and resource records reported by the operator’s DNS source.",
      steps: ["Compare request and response counters to spot query failures.", "Review zone serial and per-zone statistics for update/freshness clues.", "Filter or sort the resource record table to inspect a name, type, TTL and value."],
      recovery: "If statistics or records are unavailable, read the displayed source status first; a missing DNS snapshot is different from an empty zone.",
      context: "DNS resolves service names to network locations. Resolution does not create a ZPR authorization; service access is still decided by Visa Service and policy.",
      docs: "Control Room guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "security-review": {
      title: "Security review",
      intro: "Compare the browser-local Baseline inventory with the current observed inventory; this view does not replace authoritative policy review.",
      steps: ["Visiting Security clears the colored navigation side indicator for the alerts already observed, without dismissing their findings. A new high-priority alert colors the side indicator again; the link background stays unchanged.", "Select visible findings to inspect or dismiss them.", "Reset Baseline only when you intend to replace the saved browser snapshot."],
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
      steps: ["Browse returned directory records using LDAP tree, People, Groups or Attributes; filter without rereading the provider.", "Records load on the first visit. Automatic polling and returning to this page do not reload LDAP or collapse its tree. Click the global Refresh button here to read the provider again.", "Open a provider link only when you need its operator console."],
      recovery: "For unavailable sources, check provider health, identity mappings, and the Control-Service trust configuration.",
      context: "Providers are configured and authenticated server-side. The browser uses same-origin Control Room APIs and does not receive provider URLs, tokens, or directory credentials. Trusted attributes inform identity and policy; the provider does not grant access.",
      docs: "Trusted-service configuration", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/README.md",
    },
    "adapter-logs": {
      title: "Adapter logs",
      intro: "Read bounded tails from the operator-configured production adapter/controller inventory. Each panel heading names the adapter; the chevron tooltip identifies its stable ID and selected source.",
      steps: ["Choose Adapter logs or Controller logs to select which configured source class appears.", "Click the chevron beside an adapter name to open an expanded choice list; select an entry to change that panel. Escape or an outside click dismisses the list.", "Running only hides sources whose adapter/machine state is not running. Show all adapters opens one panel for each matching configured source; Hide all adapters closes those panels.", "Word wrap starts enabled and changes long lines across all panels, including maximized panels. Pause stops polling; Maximize expands one panel."],
      recovery: "An unavailable source is reported independently; check its configured path/container and the Control-Service log inventory.",
      context: "These are operator-owned production logs returned through Control-Service, not Simulator workload logs. A machine can be running while a particular log source is disconnected or missing.",
      docs: "Adapter log configuration", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/cmd/zpr-web-dashboard/README.md",
    },
    "zpr-config": {
      title: "ZPR configuration",
      intro: "This editor stores versioned ZPLC runtime-configuration drafts. Analyze checks TOML syntax, Format normalizes assignment spacing without reordering keys or deleting comments, and File opens/imports or downloads a local draft.",
      steps: ["Choose a saved draft and revision, or use File → New draft/Open file.", "Run Analyze before saving. TOML parser errors with a reported source line appear in the gutter; other diagnostics stay in the status message.", "Format first validates TOML; if valid it adjusts spacing around assignments while preserving comments and values.", "Save draft creates a version; it does not apply settings to a running service."],
      recovery: "Fix the marked TOML line and Analyze again. If validation reports no line, read the full status rather than assuming line 1. A saved configuration draft still needs its separate approved runtime-application workflow.",
      context: "Policy is about who may access which service; ZPLC configuration controls runtime/service settings. Editing this draft does not compile, stage or activate network policy.",
      example: "[visa_service]\ndock_node = \"node-a\"\nlisten_address = \"[::]:8080\" # retained comment",
      docs: "ZPLC and configuration guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/README.md",
    },
    policy: {
      title: "Policy editor",
      intro: "A policy describes which authenticated client identities may access which registered service identities. The editor works on versioned source records; a draft is not active runtime policy.",
      steps: ["Browse opens the policy/assertion catalog; File contains record actions such as create, save and version operations.", "Analyze compiles and tests the exact edited source against configured candidate fixtures. Markers are attached only to the source line that produced them.", "Format changes policy-source layout only; it neither analyzes nor saves.", "Stage creates a review candidate. It does not deploy or activate that candidate."],
      recovery: "Analysis unavailable means required test fixtures could not be built; it is distinct from a compiler error. Check the status message and fixture-provider configuration.",
      context: "ZPL matches authenticated client/user or device attributes and service identity attributes. Service rules describe a destination; `allow` grants matching client classes access. The browser calls same-origin `/api/*` and carries no Visa Service or Policy Repository credentials.",
      example: "define FinanceStaff as a user with user.department:Finance;\ndefine FinanceWorkspace as a service with device.zpr.adapter.cn:finance-workspace;\nservice FinanceWorkspace as json {\"service_class\":\"FinanceWorkspace\",\"actor_cn\":\"finance-workspace\",\"summary\":\"Illustrative finance reporting and ledger workspace.\",\"endpoint\":\"zpr://finance-workspace\",\"status\":\"example only; not deployed\"}.\n  allow FinanceStaff.",
      docs: "Policy authoring guide", href: "https://github.com/org-zpr/zpr-visaservice/blob/main/zpr-dashboard/README.md#control-room-and-policy-service",
    },
    assertions: {
      title: "Assertion editor",
      intro: "Assertions are report-only checks against trusted directory data. They help operators verify expected identities, group membership and attributes; they do not define an access grant.",
      steps: ["Read source loads the configured source catalog; it does not edit directory data.", "Analyze evaluates the exact unsaved assertion source and shows checks/results. Source-line warnings appear in the gutter; provider/configuration failures remain in status.", "Format normalizes assertion layout but does not save.", "Save stores a new assertion revision. Enabling periodic evaluation is separate from saving."],
      recovery: "If trusted data cannot be read, verify the provider and its organization-scoped identity mapping before interpreting assertion results.",
      context: "Assertions validate trusted directory data and never grant or deny network access. Trusted readers and credentials are configured on the server; the browser cannot submit provider URLs, credentials, or data snapshots. A warning is advisory; a failed/unavailable source means the check lacks data, not that it passed.",
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
    const topbar = document.querySelector(".main-content .top-actions, main .top-actions") || document.querySelector(".main-content .topbar, main .topbar, .topbar");
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