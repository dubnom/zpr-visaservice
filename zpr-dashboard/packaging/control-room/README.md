# Production web assets

From `zpr-dashboard`, build a new output directory:

```sh
sh packaging/control-room/build.sh ./bin/control-room-production
./bin/control-room-production/zpr-control-room -mode control-room
```

The directory contains the executable, `assets.txt` (the static manifest), and
`embedded-files.txt` (all command-embedded files, including production assistant
skills). Simulator directory/scenario/organization assistant artifacts are
excluded. The `production` Go build tag excludes Simulator documents,
page-specific scripts/styles/navigation and other unlisted files. Simulator mode
is rejected **before** any Simulator configuration, stores or listeners load.
Control Room uses this independent bundle even in the default development build.
New static files are not automatically admitted: update the explicit embed list
in `assets_control_room.go` and this manifest together. Tests compare both with
`go list -tags production`'s compiler-selected embedded files.

Shared rendering modules (including trusted-source and log views) remain because
Control Room supplies its own endpoint/data contracts. Simulator-only documents
that select those modules' lab contexts are not packaged or served on the
production origin. Operator login, change review, budget authority and the policy
browser remain production documents. No source tree, examples, simulation
manifests/profiles or lab runtime directories are copied into this package.

For Simulator, use the default build, separately from the production artifact:

```sh
go build -o ./bin/zpr-simulator ./cmd/zpr-web-dashboard
./bin/zpr-simulator -mode simulator
```

This packet separates **assets**, not the entire command's compiled dependency
graph or deployment lifecycle: the existing multipurpose command still compiles
Simulator Go code. Independent executable/package extraction and production
startup configuration remain separate release gates. Do not use the shared lab
launcher as production deployment evidence.

Validation (keep scratch directories inside the worktree):

```sh
mkdir -p .asset-validation
TMPDIR="$PWD/.asset-validation" go test ./cmd/zpr-web-dashboard -run 'Test(ProductionAsset|SimulatorOwnAssetsHTTP|ControlRoomBoundaryProcessStartup)' -count=1
TMPDIR="$PWD/.asset-validation" go test -tags production ./cmd/zpr-web-dashboard -run 'TestProductionAsset' -count=1
TMPDIR="$PWD/.asset-validation" go test -c -o .asset-validation/assets.test ./cmd/zpr-web-dashboard
TMPDIR="$PWD/.asset-validation" go test -tags production -c -o .asset-validation/assets-production.test ./cmd/zpr-web-dashboard
TMPDIR="$PWD/.asset-validation" ZPR_ASSET_BROWSER_TEST_BINARY="$PWD/.asset-validation/assets.test" ZPR_PRODUCTION_ASSET_BROWSER_TEST_BINARY="$PWD/.asset-validation/assets-production.test" npm run test:browser -- production-assets.spec.mjs control-room-boundary.spec.mjs --workers=1
python3 scripts/test-control-room-assets.py
python3 scripts/check-control-room-boundary.py # may still fail for other packets
```

For a separately supervised **evaluation-only** local Control Room/Control-Service
instance with unique TLS/IdP/LDAP/logging state, use the
[B1 evaluation runbook](../evaluation/README.md). It builds this production-tagged
command; Simulator assets/services are not needed. It is not the appliance BOM.
