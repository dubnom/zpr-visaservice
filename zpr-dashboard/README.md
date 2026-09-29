# ZPR Dashboard TUI Application

This repository contains the Zero-Trust Packet Routing (ZPR) Terminal User Interface application. The dashboard application allows you to manage your ZPR infrastructure quickly through an easy-to-understand and easy-to-use interface.

# Development

To setup your development environment, run `go mod download` to install the local dependencies. Finally, run `make run` to start the TUI.

## Project layout

```
config.toml.example  Endpoint, timeout and credential file paths
cmd/zpr-dashboard/   Program entry point (thin main)
internal/
  app/               Bubble Tea model: Init/Update/View, input handling
  pages/             Top-level tab views (Dashboard, Visas, Actors)
  components/        Reusable panels, tables, charts and their data
  charts/            Braille dot chart and bar chart primitives
  styles/            Color palette and shared lipgloss styles
  config/            Reads config.toml into the environment
  dataplane/         HTTP client for the vs-admin API
```

# Compiling the binary

To compile the final binary, run `make all`. The binaries will be located at bin/zpr-dashboard and bin/zpr-sim.


## Running it

Copy `config.toml.example` to `config.toml` and edit it.


```sh
make run                                # Start admin panel
```

## Web monitor

The read-only browser dashboard lives in `cmd/zpr-web-dashboard`. It polls the
Visa Service admin API for counters, nodes and links, actors, registered
services, active visas, and recent policy denials. Trusted-source connection
status is based on the Visa Service's last real lookup when available;
older admin APIs report it as unreported.

Configure the HTTPS admin API and read-only credentials with `ZPR_ADMIN_URL`,
`ZPR_ADMIN_CA_FILE`, and `ZPR_ADMIN_KEY_FILE`, then run:

```sh
go run ./cmd/zpr-web-dashboard
```

Open `http://127.0.0.1:8787`. Full instructions are in
[`cmd/zpr-web-dashboard/README.md`](cmd/zpr-web-dashboard/README.md).

