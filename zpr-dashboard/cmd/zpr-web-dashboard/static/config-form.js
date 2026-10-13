(() => {
  const sections = [
    ["resolver", "Resolver", { order: ["Resolution order (one per line)", "lines"] }],
    ["visa_service", "Visa Service", { dock_node: ["Dock node", "string", true] }],
    ["nodes", "Nodes", { zpr_address: ["ZPR address or resolver hostname", "string", true], provider: ["Provider attributes", "pairs", true] }],
    ["links", "Links", { peers: ["Peers (exactly two)", "peers", true], attributes: ["Link attributes", "pairs"] }],
    ["trusted_services", "Trusted services", {
      api: ["API", "string"], service: ["Service interface", "string"], client: ["Client interface", "string"],
      cert_path: ["Certificate path", "string"], prefix: ["Prefix", "string"],
      expiration_seconds: ["Expiration seconds", "integer"], returns_attributes: ["Return mappings (one per line)", "lines"],
      identity_attributes: ["Identity attributes (one per line)", "lines"], provider: ["Provider attributes", "pairs"],
    }],
    ["protocols", "Protocols", {
      l4protocol: ["Layer 4 protocol", "string", true], l7protocol: ["Layer 7 protocol", "string"],
      port: ["Ports (number, list or range)", "port"], icmp_type: ["ICMP interaction", "string"], icmp_codes: ["ICMP codes (comma-separated)", "codes"],
    }],
    ["services", "Services", {
      protocol: ["Protocol", "string", true], provider: ["Provider attributes", "pairs"],
      port: ["Port override", "port"], icmp_type: ["ICMP interaction override", "string"], icmp_codes: ["ICMP code overrides", "codes"],
    }],
  ];
  const table = value => value !== null && typeof value === "object" && !Array.isArray(value);
  const own = (object, key) => Object.hasOwn(object, key);
  const literal = value => Array.isArray(value) ? `[${value.map(literal).join(", ")}]` :
    table(value) ? `{ ${Object.entries(value).map(([key, item]) => `${JSON.stringify(key)} = ${literal(item)}`).join(", ")} }` :
      JSON.stringify(value);
  const element = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text !== undefined) node.textContent = text;
    if (className) node.className = className;
    return node;
  };

  window.ZPRConfigForm = {
    create({ source, request, context, onChange, onStateChange }) {
      const root = document.getElementById("zpr-config-form");
      const fields = document.getElementById("zpr-config-form-fields");
      const error = document.getElementById("zpr-config-form-error");
      const toggle = document.getElementById("zpr-config-mode-toggle");
      const raw = source.closest(".config-source-editor");
      let active = false;
      let busy = false;
      let invalid = false;
      let renderedContext = "";
      let generation = 0;
      let externalPending = false;
      let localDirty = false;
      const token = () => JSON.stringify(context());
      const message = text => { error.textContent = text; error.hidden = !text; };
      function controls() {
        fields.disabled = busy || externalPending;
        toggle.disabled = externalPending;
        onStateChange();
      }
      function showRaw() {
        active = false;
        generation++;
        busy = false;
        invalid = false;
        localDirty = false;
        root.hidden = true;
        raw.hidden = false;
        toggle.textContent = "Form";
        toggle.setAttribute("aria-pressed", "false");
        document.querySelector('[data-editor-search-target="zpr-config-source"]').hidden = false;
        controls();
        source.focus();
      }
      async function transform(operation = "", path = [], value = "") {
        const current = token();
        const run = ++generation;
        renderedContext = current;
        busy = true;
        if (operation) localDirty = true;
        controls();
        if (!operation) {
          fields.replaceChildren();
          message("Loading configuration form...");
        }
        try {
          const result = await request("/api/policy/config/form", { source: source.value, operation, path, value });
          if (!active || run !== generation || current !== token()) return;
          message("");
          if (operation) {
            source.value = result.source;
            onChange();
          }
          renderedContext = token();
          invalid = false;
          localDirty = false;
          const focused = document.activeElement?.dataset.configFocus;
          render(result.config);
          if (focused) fields.querySelector(`[data-config-focus="${CSS.escape(focused)}"]`)?.focus({ preventScroll: true });
        } catch (failure) {
          if (!active || run !== generation || current !== token()) return;
          invalid = true;
          if (operation) localDirty = true;
          message(failure.message);
        } finally {
          if (run === generation) {
            busy = false;
            controls();
          }
        }
      }
      function button(text, action) {
        const node = element("button", text, "button button-refresh");
        node.type = "button";
        node.addEventListener("click", action);
        return node;
      }
      function remove(path) {
        if (confirm(`Remove ${path.join(".")} from this draft, including any unfamiliar settings inside it?`)) void transform("remove", path);
      }
      function add(path, kind = "table") {
        const name = prompt(`Name for new ${path.join(".")} entry`, "");
        if (name === null) return;
        if (!name.trim() || /[\r\n\0]/.test(name)) { message("Enter a nonempty, single-line name."); return; }
        void transform(kind === "table" ? "table" : "set", [...path, name.trim()], kind === "table" ? "" : '""');
      }
      function commit(path, value, object) {
        invalid = false;
        localDirty = false;
        message("");
        if ((value === undefined && !own(object, path.at(-1))) ||
            (value !== undefined && literal(value) === literal(object[path.at(-1)]))) { controls(); return; }
        localDirty = true;
        void transform(value === undefined ? "remove" : "set", path, value === undefined ? "" : literal(value));
      }
      function fail(input, text) {
        input.setCustomValidity(text);
        input.setAttribute("aria-invalid", "true");
        invalid = true;
        message(text);
        controls();
      }
      function scalar(parent, object, path, [title, kind, required = false]) {
        const value = object[path.at(-1)];
        const shape = value === undefined || (
          kind === "lines" ? Array.isArray(value) && value.every(item => typeof item === "string" && !/[\r\n]/.test(item)) :
          kind === "codes" ? Array.isArray(value) && value.every(item => Number.isInteger(item) && item >= 0 && item <= 255) :
          kind === "integer" ? Number.isInteger(value) && value >= 0 && value <= 4294967295 :
          kind === "port" ? typeof value === "string" || Number.isInteger(value) : typeof value === "string"
        );
        const label = element("label", undefined, "config-form-field");
        label.append(element("span", title + (required ? " *" : "")));
        const input = element(kind === "lines" || (kind === "string" && /[\r\n]/.test(value ?? "")) ? "textarea" : "input");
        input.dataset.configPath = JSON.stringify(path);
        input.dataset.configFocus = JSON.stringify(path);
        input.required = required;
        if (kind === "integer") { input.type = "number"; input.min = "0"; input.max = "4294967295"; input.step = "1"; }
        input.value = value === undefined ? "" : kind === "lines" ? value.join?.("\n") ?? "" : kind === "codes" ? value.join?.(", ") ?? "" : String(value);
        if (!shape) {
          input.disabled = true;
          label.append(element("small", "This value's type requires raw TOML; it is preserved."));
        }
        input.addEventListener("input", () => {
          localDirty = true;
          input.setCustomValidity("");
          input.removeAttribute("aria-invalid");
          controls();
        });
        input.addEventListener("change", () => {
          const text = input.value;
          if (required && !text.trim()) { fail(input, `${title} is required.`); return; }
          let next = text || undefined;
          if (text && kind === "integer") {
            if (!/^\d+$/.test(text) || !input.checkValidity()) { fail(input, `${title} must be an integer from 0 to 4294967295.`); return; }
            next = Number(text);
          } else if (text && kind === "lines") {
            next = text.split(/\r?\n/).filter(line => line.trim());
          } else if (text && kind === "codes") {
            const parts = text.split(",").map(part => part.trim());
            if (!parts.every(part => /^\d+$/.test(part) && Number(part) <= 255)) { fail(input, "ICMP codes must be integers from 0 to 255."); return; }
            next = parts.map(Number);
          } else if (text && kind === "port" && /^\d+$/.test(text)) {
            next = Number(text);
          }
          if (kind === "port" && text && !text.split(",").every(part => {
            const range = part.trim().split("-");
            return range.length <= 2 && range.every(port => /^\d+$/.test(port) && Number(port) <= 65535) &&
              (range.length === 1 || Number(range[0]) <= Number(range[1]));
          })) { fail(input, "Ports must be numbers from 0 to 65535, ascending ranges, or comma-separated lists."); return; }
          commit(path, next, object);
        });
        label.append(input);
        parent.append(label);
      }
      function rows(parent, object, path, [title, kind, required = false]) {
        const value = object[path.at(-1)] ?? [];
        const valid = Array.isArray(value) && value.every(row => kind === "pairs" ?
          Array.isArray(row) && row.length === 2 && row.every(item => typeof item === "string") :
          table(row) && typeof row.node === "string" && (row.interface === undefined || typeof row.interface === "string"));
        const group = element("fieldset", undefined, "config-form-rows");
        group.dataset.incomplete = String(required && (!value.length || kind === "peers" && value.length !== 2));
        group.append(element("legend", title + (required ? " *" : "")));
        parent.append(group);
        if (!valid) { group.append(element("p", "This value's type requires raw TOML; it is preserved.")); return; }
        const update = next => commit(path, next.length ? next : undefined, object);
        value.forEach((row, index) => {
          const line = element("div", undefined, "config-form-row");
          ["first", "second"].forEach((part, column) => {
            const label = element("label");
            label.append(element("span", kind === "pairs" ? (column ? "Value" : "Attribute") : (column ? "Interface (optional)" : "Node")));
            const input = element("input");
            input.dataset.configFocus = JSON.stringify([...path, index, column]);
            input.value = kind === "pairs" ? row[column] : row[column ? "interface" : "node"] ?? "";
            input.required = kind === "pairs" || column === 0;
            input.addEventListener("input", () => {
              localDirty = true;
              input.setCustomValidity("");
              input.removeAttribute("aria-invalid");
              controls();
            });
            input.addEventListener("change", () => {
              if (input.required && !input.value.trim()) { fail(input, `${title}: enter ${column ? "a value" : "a name"}.`); return; }
              const next = structuredClone(value);
              if (kind === "pairs") next[index][column] = input.value;
              else if (column && !input.value) delete next[index].interface;
              else next[index][column ? "interface" : "node"] = input.value;
              update(next);
            });
            label.append(input);
            line.append(label);
          });
          line.append(button("Remove row", () => update(value.filter((_, rowIndex) => rowIndex !== index))));
          group.append(line);
        });
        group.append(button("Add row", () => update([...value, kind === "pairs" ? ["", ""] : { node: "" }])));
        if (required && !value.length) group.append(element("small", "Add the required entries before saving."));
      }
      function mapping(parent, path, object, title) {
        const group = element("fieldset", undefined, "config-form-mapping");
        group.append(element("legend", title));
        if (object !== undefined && !table(object)) {
          group.append(element("p", "This value's type requires raw TOML; it is preserved."));
        } else {
          for (const [name, value] of Object.entries(object ?? {})) {
            const row = element("div", undefined, "config-form-row");
            scalar(row, { [name]: value }, [...path, name], [name, "string", true]);
            row.append(button("Remove entry", () => remove([...path, name])));
            group.append(row);
          }
          group.append(button("Add entry", () => add(path, "string")));
        }
        parent.append(group);
      }
      function card(parent, path, object, schema, named, exists = true) {
        const group = element("fieldset", undefined, "config-form-card");
        group.append(element("legend", named ? path.at(-1) : sections.find(section => section[0] === path[0])?.[1]));
        group.dataset.configTable = JSON.stringify(path);
        if (!table(object)) { group.append(element("p", "This entry is not a table. Use raw TOML to edit it.")); parent.append(group); return; }
        const grid = element("div", undefined, "config-form-grid");
        for (const [key, definition] of Object.entries(schema)) {
          const [title, kind] = definition;
          const effective = path[0] === "visa_service" && !exists ? [title, kind, false] : definition;
          if (kind === "pairs" || kind === "peers") rows(grid, object, [...path, key], effective);
          else scalar(grid, object, [...path, key], effective);
        }
        group.append(grid);
        if (path[0] === "resolver") mapping(group, [...path, "hosts"], object.hosts, "Host addresses");
        if (path[0] === "nodes") mapping(group, [...path, "substrate_addrs"], object.substrate_addrs, "Substrate interfaces (HOST:PORT)");
        const known = new Set([...Object.keys(schema), ...(path[0] === "resolver" ? ["hosts"] : path[0] === "nodes" ? ["substrate_addrs"] : [])]);
        const unknown = Object.keys(object).filter(key => !known.has(key));
        if (unknown.length) group.append(element("small", `Preserved unfamiliar settings: ${unknown.join(", ")}. Edit these in raw TOML.`));
        if (named) group.append(button("Remove entry", () => remove(path)));
        else if (exists) group.append(button("Remove section", () => remove(path)));
        parent.append(group);
      }
      function render(config) {
        fields.replaceChildren();
        for (const [key, title, schema] of sections) {
          const section = element("section", undefined, "config-form-section");
          section.append(element("h3", title));
          if (key === "resolver" || key === "visa_service") card(section, [key], config[key] ?? {}, schema, false, own(config, key));
          else if (config[key] !== undefined && !table(config[key])) section.append(element("p", "This section is not a table. Use raw TOML to edit it."));
          else {
            section.append(button(`Add ${key === "trusted_services" ? "trusted service" : key.slice(0, -1)}`, () => add([key])));
            for (const [name, object] of Object.entries(config[key] ?? {})) card(section, [key, name], object, schema, true);
          }
          fields.append(section);
        }
        mapping(fields, ["bootstrap"], config.bootstrap, "Bootstrap public-key paths by certificate CN");
        const unknown = Object.keys(config).filter(key => key !== "bootstrap" && !sections.some(section => section[0] === key));
        if (unknown.length) fields.append(element("p", `Preserved unfamiliar sections: ${unknown.join(", ")}. Edit these in raw TOML.`));
      }
      toggle.addEventListener("click", () => {
        if (active) {
          if (localDirty && !confirm("Discard form input that has not been applied to TOML?")) return;
          showRaw();
          return;
        }
        active = true;
        raw.hidden = true;
        root.hidden = false;
        toggle.textContent = "Raw TOML";
        toggle.setAttribute("aria-pressed", "true");
        document.querySelector('[data-editor-search-target="zpr-config-source"]').hidden = true;
        void transform();
      });
      return {
        sync(pending) {
          externalPending = pending;
          if (active && renderedContext !== token() && !busy && !pending) void transform();
          fields.disabled = busy || pending;
          toggle.disabled = pending;
        },
        blocked: () => active && (busy || invalid || Boolean(fields.querySelector('[data-incomplete="true"]')) ||
          [...fields.querySelectorAll("input, textarea")].some(input => !input.disabled && !input.checkValidity())),
        active: () => active,
        dirty: () => active && localDirty,
        reset() {
          generation++;
          busy = false;
          invalid = false;
          localDirty = false;
          renderedContext = "";
        },
      };
    },
  };
})();
