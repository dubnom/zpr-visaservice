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
	{ key: "expires", label: "EXPIRES", value: (visa) => Number(visa.expires) || Date.parse(visa.expires || "") || 0, render: (visa) => { const value = Number(visa.expires) ? Number(visa.expires) * 1000 : Date.parse(visa.expires || ""); return Number.isFinite(value) && value > 0 ? new Date(value).toLocaleString() : "—"; } },
];
const denyColumns = [
	{ key: "source", label: "SOURCE", value: (deny) => deny.source_addr || "", render: (deny) => deny.source_addr || "—" },
	{ key: "destination", label: "DESTINATION", value: (deny) => deny.dest_addr || "", render: (deny) => deny.dest_addr || "—" },
	{ key: "protocol", label: "PROTOCOL / PORT", value: (deny) => `${deny.protocol ?? ""} ${deny.dest_port ?? ""}`, render: (deny) => `${({ 6: "TCP", 17: "UDP", 1: "ICMP" })[deny.protocol] || deny.protocol || "—"} / ${deny.dest_port ?? "—"}` },
	{ key: "reason", label: "REASON", value: (deny) => deny.code || "", render: (deny) => deny.code || "—" },
	{ key: "count", label: "HITS", value: (deny) => Number(deny.count) || 0, render: (deny) => deny.count ?? "—" },
];

function compareActivityRows(left, right, sort, columns) {
	const column = columns.find((candidate) => candidate.key === sort.key);
	const a = column?.value(left) ?? "";
	const b = column?.value(right) ?? "";
	const comparison = typeof a === "number" && typeof b === "number"
		? a - b
		: String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: "base" });
	return comparison * sort.direction;
}

function renderActivityTable(page, items, columns, emptyText) {
	const table = document.querySelector(`table[data-sort-page="${page}"]`);
	const body = table.tBodies[0];
	const sort = activitySort[page];
	const ordered = [...items].sort((left, right) => compareActivityRows(left, right, sort, columns));
	body.innerHTML = ordered.length
		? ordered.map((item) => `<tr>${columns.map((column) => `<td data-sort-cell="${column.key}" data-sort-value="${esc(column.value(item))}">${esc(column.render(item))}</td>`).join("")}</tr>`).join("")
		: `<tr><td colspan="${columns.length}" class="empty-row">${esc(emptyText)}</td></tr>`;
}

function updateActivitySortHeaders(table, page) {
	for (const header of table.querySelectorAll("th[data-sort-key]")) {
		const active = activitySort[page].key === header.dataset.sortKey;
		const direction = activitySort[page].direction === 1 ? "ascending" : "descending";
		header.setAttribute("aria-sort", active ? direction : "none");
		header.querySelector(".sort-button").setAttribute("aria-label", active
			? `Sort by ${header.textContent.trim()}, currently ${direction}`
			: `Sort by ${header.textContent.trim()}, ascending`);
	}
}

function renderActivityTables() {
	renderActivityTable("activity-visas", activityData.visas || [], visaColumns, "No visa activity yet.");
	renderActivityTable("activity-denies", activityData.denies || [], denyColumns, "No denied flows yet.");
}

for (const table of document.querySelectorAll("table[data-sort-page^='activity-']")) {
	const page = table.dataset.sortPage;
	for (const header of table.querySelectorAll("th[data-sort-key]")) {
		const button = document.createElement("button");
		button.type = "button";
		button.className = "sort-button";
		button.textContent = header.textContent;
		header.classList.add("sortable-heading");
		header.replaceChildren(button);
		header.addEventListener("click", () => {
			const previous = activitySort[page];
			activitySort[page] = {
				key: header.dataset.sortKey,
				direction: previous.key === header.dataset.sortKey ? previous.direction * -1 : 1,
			};
			updateActivitySortHeaders(table, page);
			if (activityData) renderActivityTables();
		});
	}
	updateActivitySortHeaders(table, page);
}

async function activityRefresh() {
	try {
		const response = await fetch("/api/simulator/activity");
		if (!response.ok) throw new Error(`HTTP ${response.status}`);
		activityData = await response.json();
		const stats = activityData.stats || {};
		const visas = activityData.visas || [];
		$("#requests").textContent = stats.visa_requests || "0";
		$("#approved").textContent = stats.visa_requests_approved || "0";
		$("#denied").textContent = stats.visa_requests_denied || "0";
		$("#visa-count").textContent = visas.length;
		$("#activity-state").textContent = "LIVE";
		$("#activity-time").textContent = new Date(activityData.generated_at).toLocaleTimeString();
		renderActivityTables();
	} catch {
		const status = $("#activity-state");
		if (status) status.textContent = "UNAVAILABLE";
	}
}
let activityRefreshTimer;
$("#refresh").addEventListener("click", activityRefresh);
document.addEventListener("simulator:activate", (event) => {
	if (event.detail.path !== "/activity.html" || activityRefreshTimer) return;
	activityRefresh();
	activityRefreshTimer = setInterval(activityRefresh, 4000);
});
document.addEventListener("simulator:deactivate", (event) => {
	if (event.detail.path !== "/activity.html") return;
	clearInterval(activityRefreshTimer);
	activityRefreshTimer = undefined;
});
