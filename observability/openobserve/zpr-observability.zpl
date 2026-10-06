# Reserve a dedicated adapter identity and ZPR address for OpenObserve.
define ZprObservability as a service with device.zpr.adapter.cn:zpr-observability and zpr.addr:'fd5a:5052:adda:1::54'.

# The collector uses a dedicated adapter, not the Visa Service's identity.
define TelemetryPublisher as a device with device.zpr.adapter.cn:'telemetry-publisher'.
define ObservabilityReader as a device with device.zpr.adapter.cn:ops-observer.
# The collector uses the Visa Service's read-only admin key and never installs policy.

provide VisaService at visa-admin.svc.zpr over TCP 443.
  allow TelemetryPublisher.

provide ZprObservability at observability.svc.zpr over TCP 5080.
  allow TelemetryPublisher.
  allow ObservabilityReader.
