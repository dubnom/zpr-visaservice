import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import vm from "node:vm";

const source = await readFile(new URL("../../cmd/zpr-web-dashboard/static/scenarios.js", import.meta.url), "utf8");
const context = vm.createContext({});
for (const name of ["scenarioPlanIsObject", "scenarioPlanValidateResponse", "scenarioPlanTopologyText"]) {
  const definition = source.match(new RegExp(`function ${name}\\([^]*?\\n\\}`));
  if (!definition) throw new Error(`Missing actual frontend function ${name}`);
  vm.runInContext(definition[0], context);
}
const binding = {
  version: 1, install_id: "0422ecb887a5106caea70975",
  logical_node_id: "velocity-node",
  node_container: "zpr-evaluation-0422ecb887a5106caea70975-data-plane",
  network: "zpr-evaluation-0422ecb887a5106caea70975-data-plane",
  image_id: `sha256:${"a".repeat(64)}`,
  substrate_address: "172.26.0.2", zpr_address: "fd5a:5052::2",
};
function plan() {
  return {
    status: "blocked", organization_id: "velocity", scenario_id: "velocity-single-node",
    manifest_name: "original", budget_status: "absent", topology_status: "retained-binding-verified",
    policy_status: "unresolved", northstar_assets_status: "not-required",
    package_contract_status: "retained-topology-only", plan_digest: `sha256:${"b".repeat(64)}`,
    authorization_scope_digest: `sha256:${"c".repeat(64)}`,
    machine_ids: ["machine-03", "machine-05"], workloads: [], budget_inputs: [],
    blockers: ["retained runtime machine lifecycle consumer is not installed"],
    topology: { nodes: [{ id: "velocity-node", zpr_address: binding.zpr_address, substrate: binding.substrate_address }] },
    retained_topology_binding: { ...binding },
  };
}
test("preflight displays the explicit logical-to-physical binding without implying acceptance", () => {
  const response = plan();
  expect(context.scenarioPlanValidateResponse(response, "velocity", "velocity-single-node")).toBe(response);
  const text = context.scenarioPlanTopologyText(response);
  expect(text).toContain("not scenario acceptance");
  expect(text).toContain(binding.logical_node_id);
  expect(text).toContain(binding.node_container);
  expect(text).toContain(binding.image_id);
  delete response.retained_topology_binding;
  expect(context.scenarioPlanTopologyText(response)).toBe(JSON.stringify(response.topology, null, 2));
});
test("malformed retained binding is not displayed as a valid plan", () => {
  for (const invalid of [null, {}, { ...binding, version: 2 }, { ...binding, image_id: "latest" }, { ...binding, network: "" }]) {
    const response = plan();
    response.retained_topology_binding = invalid;
    expect(() => context.scenarioPlanValidateResponse(response, "velocity", "velocity-single-node")).toThrow();
  }
});
