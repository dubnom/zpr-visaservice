import { renderColoredLog } from "/colored-log.js?v=1";

export function createLogPanel(titleText) {
  const panel = document.createElement("article");
  panel.className = "machine-log-panel";
  const header = document.createElement("header");
  const title = document.createElement("h2");
  title.textContent = titleText;
  const actions = document.createElement("div");
  actions.className = "machine-log-panel-actions";
  const maximizeButton = document.createElement("button");
  maximizeButton.className = "quiet";
  maximizeButton.type = "button";
  window.ZPRSafeDisplay.renderWindowControl(maximizeButton, false, titleText);
  const output = document.createElement("div");
  output.className = "machine-log-output";
  output.tabIndex = 0;
  output.setAttribute("aria-label", titleText);
  actions.append(maximizeButton);
  header.append(title, actions);
  panel.append(header, output);
  const view = { panel, header, title, actions, maximizeButton, output, following: true, signature: "", nextScrollTop: undefined };
  output.addEventListener("scroll", () => {
    if (output.clientHeight) view.following = output.scrollHeight - output.clientHeight - output.scrollTop <= 8;
  });
  return view;
}

export function updateLogPanel(view, source, emptyMessage, { formatJSON = false, follow = true, independentRecords = false } = {}) {
  const signature = JSON.stringify({ source, emptyMessage, formatJSON, independentRecords });
  const scrollTop = view.nextScrollTop ?? view.output.scrollTop;
  view.nextScrollTop = undefined;
  if (signature !== view.signature) {
    view.output.replaceChildren();
    if (!source || source.disconnected || source.error) {
      const status = document.createElement("p");
      status.className = "machine-log-error";
      status.setAttribute("role", "status");
      status.textContent = source?.error || (source?.disconnected ? "Disconnected" : emptyMessage);
      view.output.append(status);
    }
    if (source && (source.lines.length || !source.disconnected && !source.error)) {
      const content = document.createElement("pre");
      const lines = formatJSON ? source.lines.map(window.ZPRLogFormat.formatJSON) : source.lines;
      if (independentRecords && lines.length) {
        for (const [index, line] of lines.entries()) {
          const record = document.createElement("span");
          renderColoredLog(record, line);
          if (index) content.append("\n");
          content.append(...record.childNodes);
        }
      } else renderColoredLog(content, lines.join("\n") || "No log entries.");
      view.output.append(content);
    }
    view.signature = signature;
  }
  view.output.scrollTop = follow && view.following ? view.output.scrollHeight : scrollTop;
}

export function setLogPanelMaximized(view, maximized, label, follow = true) {
  view.panel.classList.toggle("maximized", maximized);
  window.ZPRSafeDisplay.renderWindowControl(view.maximizeButton, maximized, label);
  document.body.classList.toggle("machine-log-maximized", Boolean(document.querySelector(".machine-log-panel.maximized")));
  if (follow && view.following) view.output.scrollTop = view.output.scrollHeight;
}
