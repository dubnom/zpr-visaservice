import { readFile, writeFile } from "node:fs/promises";

const [input, output] = process.argv.slice(2);
if (!input || !output) throw new Error("Usage: node build-geography-basemap.mjs INPUT.geojson OUTPUT.svg");
const data = JSON.parse(await readFile(input, "utf8"));
const project = ([longitude, latitude]) => {
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude) || Math.abs(latitude) > 90 || Math.abs(longitude) > 180) {
    throw new Error("Invalid Natural Earth coordinate");
  }
  return `${((longitude + 180) * 5).toFixed(2)},${((90 - latitude) * 5).toFixed(2)}`;
};
const paths = data.features.flatMap(({ geometry }) => {
  if (geometry.type !== "Polygon" && geometry.type !== "MultiPolygon") throw new Error("Expected land polygons");
  const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
  return polygons.map((rings) => `<path d="${rings.map((ring) => `M${ring.map(project).join("L")}Z`).join("")}"/>`);
});
await writeFile(output, `<!-- Natural Earth 1:110m land, public domain. https://www.naturalearthdata.com/about/terms-of-use/
Source: https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_land.geojson
Generated equirectangular projection: x=(longitude+180)*5, y=(90-latitude)*5. -->
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1800 900">
<rect width="1800" height="900" fill="#122338"/>
<g fill="#34516a" stroke="#6b879f" stroke-width="1" fill-rule="evenodd">
${paths.join("\n")}
</g></svg>
`);
