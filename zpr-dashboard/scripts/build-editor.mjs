import { build } from "esbuild";
import { readFile, writeFile } from "node:fs/promises";

await build({
  entryPoints: ["browser/code-editor.js"],
  bundle: true,
  format: "iife",
  target: "es2022",
  minify: true,
  legalComments: "eof",
  outfile: "cmd/zpr-web-dashboard/static/vendor/code-editor.js",
});
const lock = JSON.parse(await readFile("package-lock.json", "utf8"));
const licenses = [];
for (const [path, entry] of Object.entries(lock.packages)) {
  if (!/^node_modules\/(@codemirror\/|@lezer\/|style-mod$|w3c-keyname$|crelt$)/.test(path)) continue;
  licenses.push(`${path.replace("node_modules/", "")} ${entry.version}\n\n${await readFile(`${path}/LICENSE`, "utf8")}`);
}
await writeFile("cmd/zpr-web-dashboard/static/vendor/CODEMIRROR-LICENSES", licenses.join("\n\n-----\n\n"));
