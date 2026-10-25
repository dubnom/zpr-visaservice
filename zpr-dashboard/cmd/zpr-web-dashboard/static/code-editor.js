(() => {
  const status = document.getElementById("code-editor-status");
  const surfaces = [...document.querySelectorAll(".config-source-editor")];
  const hooks = {};
  for (const surface of surfaces) {
    const source = surface.querySelector("textarea");
    const highlight = surface.querySelector("pre");
    const gutter = surface.querySelector(".config-source-gutter");
    if (source && highlight && gutter) {
      hooks[source.id] = { gutter: gutter.id, highlight: highlight.id };
      surface.classList.add("code-editor-loading");
    }
  }
  if (document.getElementById("policy-source")) {
    hooks["policy-source"] = { gutter: "policy-test-gutter-content", highlight: "policy-highlight", complete: completionCandidates };
    hooks["assertion-source"] = { gutter: "assertion-result-lines", highlight: "assertion-highlight" };
  }
  async function start() {
    try {
      await new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "/vendor/code-editor.js?v=9";
        script.onload = resolve;
        script.onerror = () => { script.remove(); reject(new Error("Could not load the editor. Reload to retry; no draft changes were applied.")); };
        document.head.append(script);
      });
      if (typeof hidePolicyCompletions === "function") hidePolicyCompletions();
      window.ZPRCodeEditor.initialize(hooks);
      for (const surface of surfaces) surface.classList.remove("code-editor-loading");
      if (status) status.textContent = "";
    } catch (error) {
      const message = error.message || "The editor could not start. Reload to retry.";
      if (status) status.textContent = message;
      for (const surface of surfaces) {
        const notice = document.createElement("p");
        notice.className = "code-editor-error";
        notice.setAttribute("role", "alert");
        notice.textContent = message;
        surface.append(notice);
      }
    } finally {
      document.dispatchEvent(new Event("zpr:editor-ready"));
    }
  }
  start();
})();
