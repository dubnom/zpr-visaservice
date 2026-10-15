import { Compartment, EditorSelection, EditorState, Transaction } from "@codemirror/state";
import { Decoration, EditorView, GutterMarker, gutter, keymap, lineNumbers } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab, isolateHistory } from "@codemirror/commands";
import { bracketMatching } from "@codemirror/language";
import { autocompletion, completionKeymap } from "@codemirror/autocomplete";

const instances = new Map();
// Keep the palette aligned with the color-only CSP declarations in main.go.
const tokenColors = {
  keyword: "#e5a66c", string: "#d7a7cf", comment: "#829188", number: "#d8bd80",
  class: "#92c5df", "class-definition": "#c2d88a", attribute: "#b8d78b",
  value: "#d8bd80", punctuation: "#bac9bf",
};
const theme = EditorView.theme({
  "&": { height: "100%", color: "#e4ebe5", backgroundColor: "#17211e" },
  ...Object.fromEntries(Object.entries(tokenColors).map(([token, color]) => [`.cm-token-${token}`, { color }])),
  ".cm-scroller": { overflow: "auto" },
  ".cm-content": { padding: "12px 0", caretColor: "#e6eee8" },
  ".cm-line": { padding: "0 8px" },
  ".cm-gutters": { backgroundColor: "#1d2b24", color: "#a0afa5", borderRight: "1px solid #344a40" },
  ".cm-result-gutter .cm-gutterElement": { minWidth: "46px", padding: "0 3px" },
  ".cm-cursor": { borderLeftColor: "#e6eee8" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": { backgroundColor: "#36543f !important" },
  ".cm-tooltip": { backgroundColor: "#23362b", color: "#e6eee8", border: "1px solid #6c8671" },
  ".cm-tooltip-autocomplete ul li[aria-selected]": { backgroundColor: "#36543f", color: "#fff" },
  ".cm-source-error": { borderBottom: "2px dotted #ffb49e" },
  ".cm-matchingBracket": { color: "inherit", backgroundColor: "transparent", outline: "1px solid #7ea65b" },
  ".cm-nonmatchingBracket": { color: "inherit", outline: "1px solid #ffb49e" },
}, { dark: true });

class ResultMarker extends GutterMarker {
  constructor(buttons) {
    super();
    this.buttons = buttons;
    this.signature = buttons.map(button => button.outerHTML).join("");
  }
  eq(other) { return this.signature === other.signature && this.buttons.every((button, index) => button === other.buttons[index]); }
  toDOM() {
    const container = document.createElement("span");
    for (const original of this.buttons) {
      const button = original.cloneNode(true);
      button.addEventListener("click", () => original.click());
      container.append(button);
    }
    return container;
  }
}

function mount(source, hooks) {
  const sourceStyle = getComputedStyle(source);
  const typography = EditorView.theme({
    "&": { fontSize: sourceStyle.fontSize },
    ".cm-scroller": { fontFamily: sourceStyle.fontFamily, lineHeight: sourceStyle.lineHeight },
  });
  const surface = source.parentElement;
  const host = document.createElement("div");
  host.className = "code-editor-host";
  host.dataset.source = source.id;
  const root = host.attachShadow({ mode: "open" });
  if (!root.adoptedStyleSheets || !window.CSSStyleSheet) throw new Error("This browser does not support the CodeMirror trial's CSP-safe styles. Use the current editor.");
  const styles = document.createElement("link");
  styles.rel = "stylesheet";
  styles.href = "/code-editor-trial.css?v=1";
  root.append(styles);
  surface.append(host);
  const nativeValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value");
  const nativeStart = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "selectionStart");
  const nativeEnd = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "selectionEnd");
  const nativeSelection = source.setSelectionRange;
  const controls = new Compartment();
  const wrapping = new Compartment();
  let forwarding = false;
  let destroyed = false;
  let view;
  let errorMarks = Decoration.none;
  const resultRows = document.getElementById(hooks.gutter);
  const highlight = document.getElementById(hooks.highlight);

  function forwardKey(event) {
    if (!(event.ctrlKey || event.metaKey) || event.altKey || !["s", "f", "h"].includes(event.key.toLowerCase())) return false;
    const forwarded = new KeyboardEvent("keydown", {
      key: event.key, code: event.code, ctrlKey: event.ctrlKey, metaKey: event.metaKey,
      shiftKey: event.shiftKey, bubbles: true, cancelable: true,
    });
    source.dispatchEvent(forwarded);
    if (forwarded.defaultPrevented) { event.preventDefault(); return true; }
    return false;
  }
  function editable() {
    return [EditorState.readOnly.of(source.disabled || source.readOnly), EditorView.editable.of(!source.disabled && !source.readOnly)];
  }
  function completion(context) {
    if (source.id !== "policy-source") return null;
    const result = hooks.complete(context.state.doc.toString(), context.pos);
    if (!context.explicit && !result.word) return null;
    return { from: result.start, to: result.end, options: result.items.map(label => ({ label, type: "keyword" })) };
  }
  const extensions = [
    history(), theme, typography, lineNumbers(), bracketMatching(),
    controls.of(editable()), wrapping.of(source.wrap === "off" ? [] : EditorView.lineWrapping),
    autocompletion({ override: [completion] }),
    keymap.of([...completionKeymap, ...defaultKeymap, ...historyKeymap, indentWithTab]),
    EditorState.transactionFilter.of(transaction => {
      const limit = source.maxLength;
      return transaction.docChanged && limit >= 0 && transaction.newDoc.length > limit && transaction.newDoc.length > transaction.startState.doc.length ? [] : transaction;
    }),
    EditorView.contentAttributes.of({
      "aria-label": source.getAttribute("aria-label"),
      "aria-describedby": source.getAttribute("aria-describedby") || "",
      spellcheck: "false",
    }),
    EditorView.domEventHandlers({ keydown: forwardKey }),
    gutter({
      class: "cm-result-gutter",
      lineMarker(editor, line) {
        const number = editor.state.doc.lineAt(line.from).number;
        const buttons = [...(resultRows.querySelector(`[data-line="${number}"]`)?.querySelectorAll("button") || [])];
        return buttons.length ? new ResultMarker(buttons) : null;
      },
      lineMarkerChange: update => update.transactions.some(transaction => transaction.annotation(Transaction.userEvent) === "results"),
    }),
    EditorView.decorations.of(() => errorMarks),
    EditorView.updateListener.of(update => {
      if (update.docChanged) {
        nativeValue.set.call(source, update.state.doc.toString());
        syncSelection();
        forwarding = true;
        try { source.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText" })); }
        finally { forwarding = false; }
      } else if (update.selectionSet) syncSelection();
    }),
  ];
  function syncSelection() {
    const { from, to } = view.state.selection.main;
    nativeSelection.call(source, from, to);
  }
  const createState = text => EditorState.create({ doc: text, extensions });
  try { view = new EditorView({ state: createState(source.value), parent: root, root }); }
  catch (error) { host.remove(); throw error; }
  const descriptors = {};
  function property(name, descriptor) {
    descriptors[name] = Object.getOwnPropertyDescriptor(source, name);
    Object.defineProperty(source, name, { configurable: true, ...descriptor });
  }
  property("value", {
    get: () => nativeValue.get.call(source),
    set(value) {
      nativeValue.set.call(source, value);
      const text = nativeValue.get.call(source);
      errorMarks = Decoration.none;
      view.setState(createState(text));
    },
  });
  for (const [name, native] of [["selectionStart", nativeStart], ["selectionEnd", nativeEnd]]) {
    property(name, {
      get: () => native.get.call(source),
      set(value) {
        native.set.call(source, value);
        select(source.selectionStart, source.selectionEnd);
      },
    });
  }
  function select(start, end, direction = "forward") {
    nativeSelection.call(source, start, end, direction);
    const from = source.selectionStart, to = source.selectionEnd;
    view.dispatch({ selection: EditorSelection.single(direction === "backward" ? to : from, direction === "backward" ? from : to), scrollIntoView: true });
  }
  property("setSelectionRange", { value: select });
  property("focus", { value: () => view.focus() });
  function externalInput() {
    if (forwarding) return;
    if (source.value !== view.state.doc.toString()) view.setState(createState(source.value));
  }
  source.addEventListener("input", externalInput);
  const attributes = new MutationObserver(() => {
    view.dispatch({ effects: [
      controls.reconfigure(editable()),
      wrapping.reconfigure(source.wrap === "off" ? [] : EditorView.lineWrapping),
    ] });
  });
  attributes.observe(source, { attributes: true, attributeFilter: ["disabled", "readonly", "wrap"] });
  function resultsChanged() {
    if (destroyed) return;
    const ranges = [];
    const walker = document.createTreeWalker(highlight, NodeFilter.SHOW_TEXT);
    let offset = 0, node;
    while ((node = walker.nextNode())) {
      const length = node.textContent.length;
      const token = [...node.parentElement.classList].find(name => (name.startsWith("zpl-") && name !== "zpl-error") || name.startsWith("assertion-token-"));
      const kind = token?.replace(/^(zpl-|assertion-token-)/, "");
      if (kind && tokenColors[kind] && length && offset < view.state.doc.length) {
        ranges.push(Decoration.mark({ class: `cm-token-${kind}` }).range(offset, Math.min(offset + length, view.state.doc.length)));
      }
      if (node.parentElement.closest(".zpl-error") && length && offset < view.state.doc.length) {
        ranges.push(Decoration.mark({ class: "cm-source-error" }).range(offset, Math.min(offset + length, view.state.doc.length)));
      }
      offset += length;
    }
    errorMarks = Decoration.set(ranges, true);
    view.dispatch({ annotations: Transaction.userEvent.of("results") });
    view.dom.querySelector(".cm-gutters")?.removeAttribute("aria-hidden");
  }
  const results = new MutationObserver(resultsChanged);
  results.observe(resultRows, { childList: true, subtree: true, attributes: true, attributeFilter: ["title", "disabled", "data-effect", "data-state"] });
  results.observe(highlight, { childList: true, subtree: true });
  surface.classList.add("code-editor-active");
  resultsChanged();
  return {
    host,
    applyEdits(edits) {
      if (source.disabled || source.readOnly) throw new Error("This editor is read-only.");
      if (!Array.isArray(edits) || edits.length > 100) throw new Error("AI edits must be a bounded list of exact replacements.");
      const text = view.state.doc.toString();
      const changes = edits.map(edit => {
        if (!edit || typeof edit.before !== "string" || typeof edit.after !== "string") throw new Error("Each AI edit requires exact before and after text.");
        const from = text.indexOf(edit.before);
        if ((!edit.before && text) || from < 0 || (edit.before && text.indexOf(edit.before, from + 1) >= 0)) throw new Error("AI edit text is missing or ambiguous. Ask again using the current draft.");
        return { from, to: from + edit.before.length, insert: edit.after };
      }).sort((a, b) => a.from - b.from);
      for (let index = 1; index < changes.length; index++) {
        if (changes[index].from < changes[index - 1].to || changes[index].from === changes[index - 1].from) throw new Error("AI edits overlap. No changes were applied.");
      }
      const length = changes.reduce((length, change) => length + change.insert.length - (change.to - change.from), text.length);
      if (source.maxLength >= 0 && length > source.maxLength) throw new Error("The AI edit exceeds the editor size limit.");
      if (changes.length) view.dispatch({
        changes, annotations: [Transaction.userEvent.of("input.ai"), isolateHistory.of("full")],
      });
    },
    replace(text, all) {
      if (source.disabled || source.readOnly) throw new Error("This editor is read-only.");
      const from = all ? 0 : view.state.selection.main.from;
      const to = all ? view.state.doc.length : view.state.selection.main.to;
      if (source.maxLength >= 0 && view.state.doc.length - (to - from) + text.length > source.maxLength) throw new Error("The edit exceeds the editor size limit.");
      view.dispatch({ changes: { from, to, insert: text }, selection: { anchor: from + text.length }, scrollIntoView: true });
      view.focus();
    },
    destroy() {
      destroyed = true;
      const selection = view.state.selection.main;
      attributes.disconnect();
      results.disconnect();
      source.removeEventListener("input", externalInput);
      view.destroy();
      host.remove();
      for (const [name, descriptor] of Object.entries(descriptors)) {
        if (descriptor) Object.defineProperty(source, name, descriptor);
        else delete source[name];
      }
      nativeSelection.call(source, selection.from, selection.to);
      surface.classList.remove("code-editor-active");
      source.dispatchEvent(new Event("scroll"));
    },
  };
}

window.ZPRCodeEditor = {
  enable(hooks) {
    try {
      for (const [id, options] of Object.entries(hooks)) {
        const source = document.getElementById(id);
        if (!instances.has(source)) instances.set(source, mount(source, options));
      }
    } catch (error) {
      this.disable();
      throw error;
    }
  },
  disable() {
    for (const instance of instances.values()) instance.destroy();
    instances.clear();
  },
  active: source => instances.has(source),
  visible: source => Boolean(instances.get(source)?.host.getClientRects().length),
  edit(source, text, all) {
    const instance = instances.get(source);
    if (!instance) return false;
    instance.replace(text, all);
    return true;
  },
  applyEdits(source, edits) {
    const instance = instances.get(source);
    if (!instance) throw new Error("Direct AI editing requires the CodeMirror trial. Ask again after enabling it.");
    instance.applyEdits(edits);
  },
};
