(() => {
  const toggle = document.getElementById("code-editor-trial");
  const status = document.getElementById("code-editor-trial-status");
  let loading;
  function loadEditor() {
    if (window.ZPRCodeEditor) return Promise.resolve();
    if (!loading) loading = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = "/vendor/code-editor.js?v=3";
      script.onload = resolve;
      script.onerror = () => { script.remove(); loading = null; reject(new Error("Could not load CodeMirror. The current editor is still available.")); };
      document.head.append(script);
    });
    return loading;
  }
  toggle.addEventListener("change", async () => {
    toggle.disabled = true;
    status.textContent = "";
    try {
      if (toggle.checked) {
        await loadEditor();
        hidePolicyCompletions();
        window.ZPRCodeEditor.enable({
          "policy-source": { gutter: "policy-test-gutter-content", highlight: "policy-highlight", complete: completionCandidates },
          "assertion-source": { gutter: "assertion-result-lines", highlight: "assertion-highlight" },
        });
        status.textContent = "CodeMirror trial on. Save and Analyze are unchanged.";
      } else {
        window.ZPRCodeEditor?.disable();
        status.textContent = "Current editor restored. Unsaved text is preserved.";
      }
    } catch (error) {
      toggle.checked = false;
      status.textContent = error.message || "CodeMirror could not start. Switch to the current editor.";
    } finally {
      toggle.disabled = false;
    }
  });
})();
