(() => {
  const status = document.getElementById("code-editor-status");
  async function start() {
    try {
      await new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "/vendor/code-editor.js?v=7";
        script.onload = resolve;
        script.onerror = () => { script.remove(); reject(new Error("Could not load the editor. Reload to retry; no draft changes were applied.")); };
        document.head.append(script);
      });
      hidePolicyCompletions();
      window.ZPRCodeEditor.initialize({
        "policy-source": { gutter: "policy-test-gutter-content", highlight: "policy-highlight", complete: completionCandidates },
        "assertion-source": { gutter: "assertion-result-lines", highlight: "assertion-highlight" },
      });
      status.textContent = "";
    } catch (error) {
      status.textContent = error.message || "The editor could not start. Reload to retry.";
    } finally {
      document.dispatchEvent(new Event("zpr:editor-ready"));
    }
  }
  start();
})();
