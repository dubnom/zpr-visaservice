# World Map layout heuristics

Current implementation reference, recorded 2026-10-10 UTC.
This document is editable: changes here do not automatically change the layout
code or deployed behavior. Mark proposed changes separately from current rules.

Implementation: [app.js](zpr-dashboard/cmd/zpr-web-dashboard/static/app.js),
primarily `renderTopology`, `geographicAdapterFanout`, `graphContentBounds`
and `setupGraphControls`.

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

## 5. Unconnected adapter placement

- Measure the occupied bounds of already placed actors, including service
  rings, gateway clouds and a label-width allowance.
- Pack unconnected adapters as a separate group, rather than putting them
  into the distant node staging area.
- Use a common group cell extent equal to the largest unconnected footprint,
  with a minimum of 64.
- Leave 40 layout units between occupied bounds and the group.
- Leave 40 units between adjacent cells:

```text
cell spacing = 2 * group extent + 40
```

- Try every column count from 1 through the number of unconnected adapters.
- For each column count, try placement to the right of the occupied bounds
  and below them, centered along the other axis.
- Select the arrangement minimizing:

```text
max(combined bounds width / viewport aspect, combined bounds height)
```

- If there are no placed actors, center the group at basemap position
  (1800, 900).
- These placements imply neither geographic coordinates nor network links.

## 6. Resize, Fit and camera behavior

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
