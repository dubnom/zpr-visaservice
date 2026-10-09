var $ = (selector) => document.querySelector(selector);
var esc = (value) => String(value ?? "—").replace(/[&<>\"]/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;" }[character]));

const activitySort = {
	"activity-visas": { key: "expires", direction: -1 },
	"activity-denies": { key: "count", direction: -1 },
};
let activityData;

const visaColumns = [
	{ key: "flow", label: "FLOW", value: (visa) => `${visa.source_addr || ""} ${visa.dest_addr || ""}`, render: (visa) => `${visa.source_addr || "—"} → ${visa.dest_addr || "—"}` },
	{ key: "proto", label: "PROTO", value: (visa) => visa.proto || visa.protocol || "", render: (visa) => visa.proto || visa.protocol || "—" },
	{ key: "port", label: "PORT", value: (visa) => Number(visa.dest_port) || 0, render: (visa) => visa.dest_port ?? "—" },
	{ key: "policy", label: "POLICY", value: (visa) => visa.policy_id || "", render: (visa) => visa.policy_id || "—" },
	{ key: "expires", label: "EXPIRES", value: (visa) => Number(visa.expires) || Date.parse(visa.expires || "") || 0, render: (visa) => { const value = Number(visa.expires) ? Number(visa.expires) * 1000 : Date.parse(visa.expires || ""); return Number.isFinite(value) && value > 0 ? window.ZPRSafeDisplay.formatDateTime(value) : "—"; } },
];
const denyColumns = [
	{ key: "source", label: "SOURCE", value: (deny) => deny.source_addr || "", render: (deny) => deny.source_addr || "—" },
	{ key: "destination", label: "DESTINATION", value: (deny) => deny.dest_addr || "", render: (deny) => deny.dest_addr || "—" },
	{ key: "protocol", label: "PROTOCOL / PORT", value: (deny) => `${deny.protocol ?? ""} ${deny.dest_port ?? ""}`, render: (deny) => `${window.ZPRSafeDisplay.protocolName(deny.protocol) || deny.protocol || "—"} / ${deny.dest_port ?? "—"}` },
	{ key: "reason", label: "REASON", value: (deny) => deny.code || "", render: (deny) => deny.code || "—" },
	{ key: "count", label: "HITS", value: (deny) => Number(deny.count) || 0, render: (deny) => deny.count ?? "—" },
];

function compareActivityRows(left, right, sort, columns) {
	const column = columns.find((candidate) => candidate.key === sort.key);
	const a = column?.value(left) ?? "";
	const b = column?.value(right) ?? "";
	const comparison = window.ZPRSortableTable.compareValues(a, b);
	return comparison * sort.direction;
}

function renderActivityTable(page, items, columns, emptyText) {
	const table = document.querySelector(`table[data-sort-page="${page}"]`);
	const body = table.tBodies[0];
	const sort = activitySort[page];
	const ordered = [...items].sort((left, right) => compareActivityRows(left, right, sort, columns));
	if (!ordered.length) {
	  window.ZPRSortableTable.renderEmptyRow(body, columns.length, emptyText);
	  return;
	}
	body.innerHTML = ordered.map((item) =>
	  `<tr>${columns.map((column) => `<td data-sort-cell="${column.key}" data-sort-value="${esc(column.value(item))}">${esc(column.render(item))}</td>`).join("")}</tr>`,
	).join("");
}

function renderActivityTables() {
	renderActivityTable("activity-visas", activityData.visas || [], visaColumns, "No visa activity yet.");
	renderActivityTable("activity-denies", activityData.denies || [], denyColumns, "No denied flows yet.");
}

for (const table of document.querySelectorAll("table[data-sort-page^='activity-']")) {
	const page = table.dataset.sortPage;
	window.ZPRSortableTable.bindSortableHeaders({
		table,
		getSort: () => activitySort[page],
		onSort: (sort) => {
			activitySort[page] = sort;
			if (activityData) renderActivityTables();
		},
	});
}

async function activityRefresh({ signal, isCurrent }) {
	const response = await fetch("/api/simulator/activity", { signal });
	if (!response.ok) throw new Error(`HTTP ${response.status}`);
	const nextData = await response.json();
	if (!isCurrent()) return;
	activityData = nextData;
	const stats = activityData.stats || {};
	const visas = activityData.visas || [];
	$("#requests").textContent = stats.visa_requests || "0";
	$("#approved").textContent = stats.visa_requests_approved || "0";
	$("#denied").textContent = stats.visa_requests_denied || "0";
	$("#visa-count").textContent = visas.length;
	$("#activity-state").textContent = "LIVE";
	$("#activity-time").textContent = window.ZPRSafeDisplay.formatTime(activityData.generated_at);
	renderActivityTables();
}
const activityPoller = window.ZPRSimulatorNavigation.createPagePoller({
	path: "/activity.html",
	run: activityRefresh,
	interval: 4000,
	onError: () => {
		const status = $("#activity-state");
		if (status) status.textContent = "UNAVAILABLE";
	},
});
$("#refresh").addEventListener("click", () => void activityPoller.refresh());
