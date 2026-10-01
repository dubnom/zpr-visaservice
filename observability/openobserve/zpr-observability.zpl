# Reserve a dedicated adapter identity and ZPR address for OpenObserve.
define ZprObservability as a service with device.zpr.adapter.cn:zpr-observability and zpr.addr:'fd5a:5052:adda:1::54'.
provide ZprObservability at observability.svc.zpr over TCP 5080.

# These are ZPR network gates; OpenObserve must also authenticate every request.
define VisaTelemetryPublisher as a device with device.zpr.adapter.cn:'vs.zpr'.
define ObservabilityReader as a device with device.zpr.adapter.cn:ops-observer.
allow VisaTelemetryPublisher to access ZprObservability.
allow ObservabilityReader to access ZprObservability.