import { AnsiUp } from "/ansi_up.js?v=6.0.6";

export function renderColoredLog(target, text) {
  const ansi = new AnsiUp();
  ansi.escape_html = true;
  const parsed = document.createElement("template");
  parsed.innerHTML = ansi.ansi_to_html(text).replaceAll(' style="', ' data-ansi-style="');
  const stylesheet = new CSSStyleSheet();
  const copy = (node) => {
    if (node.nodeType === 3) return document.createTextNode(node.textContent);
    const span = document.createElement("span");
    stylesheet.replaceSync(`span {${node.getAttribute("data-ansi-style") || ""}}`);
    const style = stylesheet.cssRules[0].style;
    for (const property of ["color", "backgroundColor", "fontWeight", "fontStyle", "textDecoration", "opacity"]) {
      if (style[property]) span.style[property] = style[property];
    }
    span.append(...[...node.childNodes].map(copy));
    return span;
  };
  target.replaceChildren(...[...parsed.content.childNodes].map(copy));
}
