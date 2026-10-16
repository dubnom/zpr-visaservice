import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const origin = "https://simulator.example.test";
const destination = "https://control.company.example/";
const assets = new URL("../../cmd/zpr-web-dashboard/static/", import.meta.url);
const html = `<!doctype html><html><head>
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/sidebar.css">
<script src="/page-runtime.js" defer></script><script src="/simulator-nav.js" defer></script>
</head><body><div class="app-shell">
<aside class="sidebar"><nav class="primary-nav" aria-label="Simulator navigation">
<a class="nav-link" href="/activity.html">Activity</a>
</nav></aside><main class="main-content"><header class="topbar"><div class="topbar-status"></div></header>
<label>Unsaved Simulator draft<input aria-label="Unsaved Simulator draft"></label>
</main></div></body></html>`;

async function setup(page, context, configResponse) {
  const requests = [];
  await context.route("**/*", async route => {
    const request = route.request();
    const url = new URL(request.url());
    requests.push({ url: request.url(), method: request.method(), headers: request.headers() });
    if (url.origin === new URL(destination).origin) {
      await route.fulfill({ contentType: "text/html", body: "<h1>Independent Control Room sign-in</h1>" });
    } else if (url.origin !== origin) {
      await route.abort();
    } else if (url.pathname === "/simulator-navigation.json") {
      await route.fulfill(configResponse);
    } else if (url.pathname === "/auth/operator/config") {
      await route.fulfill({ json: { enabled: false } });
    } else if (url.pathname === "/api/simulator/scenarios") {
      await route.fulfill({ json: { active_organization_id: "lab-only", organization: { id: "lab-only", name: "Lab only" }, scenarios: [], run: { state: "idle" } } });
    } else if (url.pathname.endsWith(".js") || url.pathname.endsWith(".css")) {
      await route.fulfill({
        contentType: url.pathname.endsWith(".js") ? "application/javascript" : "text/css",
        body: await readFile(new URL(url.pathname.slice(1), assets), "utf8"),
      });
    } else {
      await route.fulfill({ contentType: "text/html", body: html });
    }
  });
  await page.goto(`${origin}/activity.html?organization=lab-only&token=tab-only-secret#draft-source`);
  await expect(page.locator("[data-control-room-navigation]")).toBeVisible();
  return requests;
}

test("configured Control Room opens independently without handing off Simulator context", async ({ page, context }) => {
  const requests = await setup(page, context, { json: { control_room_url: destination } });
  const link = page.getByRole("link", { name: "Control Room (opens in a separate tab)", exact: true });
  await expect(link).toHaveAttribute("href", destination);
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  await expect(link).not.toHaveAttribute("data-simulator-nav");
  await page.getByRole("textbox", { name: "Unsaved Simulator draft" }).fill("unsaved source kept in Simulator");
  await page.evaluate(() => sessionStorage.setItem("navigation-test-context", "lab-only"));
  const originalURL = page.url();
  const opened = context.waitForEvent("page");
  await link.click();
  const room = await opened;
  await expect(room.getByRole("heading", { name: "Independent Control Room sign-in" })).toBeVisible();
  await expect(room).toHaveURL(destination);
  expect(await room.evaluate(() => window.opener === null)).toBe(true);
  expect(await room.evaluate(() => sessionStorage.getItem("navigation-test-context"))).toBeNull();
  await expect(page).toHaveURL(originalURL);
  await expect(page.getByRole("textbox", { name: "Unsaved Simulator draft" })).toHaveValue("unsaved source kept in Simulator");
  const roomRequests = requests.filter(request => new URL(request.url).origin === new URL(destination).origin);
  expect(roomRequests).toHaveLength(1);
  expect(roomRequests[0].headers.referer).toBeUndefined();
  expect(roomRequests[0].headers.authorization).toBeUndefined();
  expect(roomRequests[0].headers.cookie).toBeUndefined();
  expect(roomRequests[0].url).toBe(destination);
  expect(requests.filter(request => request.method !== "GET")).toEqual([]);
  await room.close();
});

for (const [name, configResponse] of [
  ["missing", { status: 503, json: { error: "Control Room navigation is not configured." } }],
  ["invalid server configuration", { status: 422, json: { error: "Invalid navigation configuration." } }],
  ["unavailable config endpoint", { status: 503, body: "unavailable" }],
  ["malformed JSON", { contentType: "application/json", body: "{" }],
  ["missing destination field", { json: {} }],
  ...[
    "/relative", "//control.company.example", "javascript:alert(1)", "data:text/html,secret",
    "http://remote.example", "https://user:secret@control.company.example/",
    `${destination}?token=secret`, `${destination}#source-secret`, `${destination}?`,
    `${destination}#`, `${destination}organization/lab-only`, `${destination} private`,
  ].map(value => [value, { json: { control_room_url: value } }]),
]) {
  test(`Control Room has no destination when navigation configuration is ${name}`, async ({ page, context }) => {
    const requests = await setup(page, context, configResponse);
    const item = page.locator("[data-control-room-navigation]");
    await expect(item).toHaveText("Control Room unavailable: configuration needed.");
    await expect(item).toHaveAttribute("title", /deployment administrator.*ZPR_SIMULATOR_CONTROL_ROOM_URL/);
    await expect(item).not.toHaveAttribute("href");
    await expect(item).not.toHaveAttribute("data-simulator-nav");
    await expect(page.getByRole("link", { name: /Control Room/ })).toHaveCount(0);
    expect(requests.every(request => new URL(request.url).origin === origin)).toBe(true);
  });
}

test("Control Room external navigation stays separate after Simulator hash navigation", async ({ page, context }) => {
  await setup(page, context, { json: { control_room_url: destination } });
  await expect(page.getByRole("link", { name: /Control Room/ })).toHaveAttribute("href", destination);
  await page.evaluate(() => { location.hash = "#changed-lab-context"; });
  const link = page.getByRole("link", { name: /Control Room/ });
  await expect(link).toHaveAttribute("href", destination);
  await expect(link).not.toHaveAttribute("data-simulator-nav");
  await expect(link).not.toHaveAttribute("aria-current");
});
