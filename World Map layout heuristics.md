# World Map layout heuristics

Current implementation reference, recorded 2026-10-10 UTC.
This document is editable: changes here do not automatically change the layout
code or deployed behavior. Mark proposed changes separately from current rules.

Implementation: [app.js](zpr-dashboard/cmd/zpr-web-dashboard/static/app.js),
primarily `renderTopology`, `geographicAdapterFanout`, `graphContentBounds`
and `setupGraphControls`.

## Packet volume overlay

- Both maps share a Traffic Off / Packets selector, initially set to Packets.
- Node halos indicate substrate RX + TX packet I/O across fastpath workers.
  RX uses `inbound_packets_received`; TX uses `outbound_packets_sent`.
  Actor-side counters are not added again. Substrate counters include control
  traffic and are not a measure of unique application packets.
- Rates use exact unsigned 64-bit counter deltas divided by exporter sample
  elapsed time, not browser refresh time. Repeated samples retain the last rate.
- Fixed bands are 0, below 100, 100–999, 1,000–9,999, and at least 10,000
  packets/sec. Halo thickness uses a five-second exponential smoothing time
  constant; zero becomes a thin neutral outline immediately. Numeric RX/TX
  details always show the latest unsmoothed rates.
- Dashed neutral outlines mean waiting, unavailable, or stale telemetry, with
  the specific reason on hover and in the inspector. Samples expire after ten
  seconds, matching the production node-counter freshness ceiling, including
  while polling is paused. A thirty-second refresh setting will therefore show
  stale intervals; use the three- or five-second refresh for live traffic.
- Counter resets, worker inventory/address changes, gaps over sixty seconds,
  disconnected telemetry, node removal, and organization changes require a new
  baseline. No negative deltas or outage-spanning rate spikes are displayed.
- The node inspector includes RX/TX, sample age, and up to twelve valid rate
  samples in a time-spaced recent-history sparkline. Its vertical scale is local
  to that history; the map bands remain fixed across nodes and maps.
- Halos are static, preserve health/denial colors, and do not change node
  positions, camera behavior, or connector endpoints. There is no inferred
  adapter/link traffic or summed map-wide throughput.
- Telemetry comes only from the existing operator snapshot; Simulator is not
  required.

## 1. Geographic node anchors

- Nodes with valid latitude/longitude are projected onto the 3600 x 1800 basemap.
- Viewport shape does not move geographic anchors.
- Nodes without coordinates use a separate layout area beyond the map's right
  edge, starting at x=4500. Their positions are not inferred geography.
- Actors and attached adapters are ordered by CN. Retained node slots provide
  placement continuity across refreshes.

## 2. Component footprints and dock lengths

Estimate each component's footprint from:

- Actor radius: node 54; gateway adapter 42; Visa adapter 39; other adapter 32.
- Service-ring radius: at least 64 when services exist, or larger when the
  combined service badge widths and spacing require it.
- Additional gateway-cloud allowance: 160.
- Adapter layout allowance: 18.

Service badge width depends on the service name, bounded between 46 and 132,
with 14 units of spacing per badge.

Each geographic adapter starts with its own minimum dock radius:

```text
minimum radius = node radius + adapter footprint + 20
```

Dock lengths therefore differ: a gateway with a service ring/cloud can sit
farther from its node than a small adapter.

## 3. Viewport-aware ring placement

When a node has no inter-node link directions, first try a full adapter ring:

- Landscape starts along the horizontal axis.
- Portrait starts along the vertical axis.
- Stretch the preferred axis by:

```text
stretch = min(1.2, sqrt(max(viewport aspect, 1 / viewport aspect)))
```

- Require every pair of adapter footprints to have at least 20 units of
  separation.
- Require all adapter footprints to remain inside the basemap.
- If this ring fails those checks, proceed to sector/arc placement.

Viewport aspect is the actual graph viewport width divided by its height,
not the browser window's nominal dimensions.

## 4. Link-free sector and arc placement

### Candidate sectors

- Sort the directions of inter-node links around the node.
- Treat the gaps between those directions as candidate sectors.
- With no link directions and an unsuitable full ring, use a 180-degree sector
  facing toward the center of the basemap.
- Usable arc angle is:

```text
min(180 degrees, 75% of sector width)
```

### Component separation

- Use the cosine rule to calculate pairwise angular separation for unequal
  footprints at unequal dock radii.
- Compare each adapter against all preceding adapters, not just its neighbor.
- Include angular allowance for the footprints at the arc ends.
- Reserve link-corridor clearance of `min(0.65 radians, sector width / 4)`
  when network links exist.
- If an arc cannot fit, multiply all candidate radii by 1.15 and retry.
- Try at most 48 radius scales per sector.

### Candidate orientations

Consider:

- Minimum and maximum allowed center angles.
- Their midpoint.
- Viewport-preferred axes, clamped into the allowable range.
- The allowable angle nearest the direction toward the basemap center.

### Scoring

Measure the bounding width and height of the node plus candidate adapters:

```text
aspect penalty = abs(log(cluster width / cluster height / viewport aspect))

preference =
    sector width * 1,000,000
  - aspect penalty * 10,000
  - abs(center angle - sector midpoint) * 1,000
```

For candidates contained within the basemap:

```text
score = preference + minimum clearance from map edges
```

For outside-map fallback candidates:

```text
fallback score = preference - largest dock radius
```

Wider link-free sectors dominate the scoring. Viewport aspect is a secondary
preference, not a hard horizontal/vertical constraint.

Stop growing a sector's radii after finding a contained placement at a scale.
Choose the highest-scoring contained candidate across sectors. If none is
contained, use the highest-scoring outside-map fallback without moving the
geographic node anchor.

## 5. Detached and Lost adapter groups

- Both maps show **Detached** and **Lost** text buttons as fixed bottom-right
  overlays, styled like Fit and zoom controls. Each displays its adapter
  count and opens a member inspector by click, Enter or Space.
- Member lists show addresses and registered service names with actor-detail
  actions. Individual service rings and gateway clouds do not enter map bounds
  until their adapter attaches. Overlay buttons are outside the SVG and do not
  affect layout, map bounds or Fit; they remain fixed while panning and zooming.
- Both groups remain visible with zero counts when inventory exists. Truly empty
  inventory retains the empty state.
- Nodes, including isolated nodes and disconnected node networks, remain visible.
- Reported docks move adapters onto the map; removal moves them into **Lost**.
- With normal motion enabled, attaching an existing Detached adapter is an
  arrival: the adapter and its services grow from their parents. Detachment
  retracts the components and their connection lines toward the surviving parent,
  accounting for layout changes. Exit layers do not affect Fit.
- Grant pulses compare both active visas and the bounded recent-visa list, so
  grants outside the ten recent entries still highlight the requester and matching
  service. Unchanged snapshots do not replay completed pulses. Reduced-motion
  preferences continue to suppress component travel and retain decision feedback.
- **Lost** means an attachment was previously observed in this browser
  for the same organization. **No attachment observed** does not claim the adapter
  has never connected. Unavailable snapshots explicitly mark attachment unknown.
- This is presentation only: no invented links, inferred coordinates, or manual
  attachment actions.

## 6. Resize, Fit and camera behavior

Both maps share a Maximize/Restore window control. Maximizing fills the browser
viewport while retaining map selection, legend, attachment groups and graph controls.
The existing resize observer recalculates layout and Auto-fit for the new space;
manual cameras remain governed by the existing Auto-fit setting. Restore or
Escape returns to the normal layout; an open component inspector closes before
Escape restores the map. Leaving the map page clears maximization.

- Re-layout when the graph viewport's aspect change exceeds:

```text
abs(log(new aspect / layout aspect)) > 0.1
```

  This corresponds to roughly a 10% aspect change.
- Fit measures rendered component bounds, excluding the geographic basemap
  and exiting animation layer.
- Bounds receive padding of `5 / 90` of their width and height on each side.
- Fit uses the actual viewport aspect ratio.
- Manual camera navigation and Auto-fit remain separate.
- Map and World Map retain separate camera/Auto-fit state when switching views.

## 7. Current limitations

- Packing is local to each node, not a global layout optimization.
- There is no global guarantee that adapter clusters belonging to different
  nodes cannot overlap.
- Viewport orientation preferences can lose to link-sector constraints.
- Dense or edge-constrained fan-out may extend beyond the basemap.
- Missing-geography nodes remain in a separate staging area.
- Footprints are heuristic estimates, not exact collision geometry for every
  rendered shape.

## Proposed changes

Use this section for desired changes; the sections above describe current code.

- [ ] No proposed changes recorded yet.
