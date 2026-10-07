define PolicyService as service with device.zpr.adapter.cn:'vs.zpr'.
define ControlService as service with device.zpr.adapter.cn:'vs.zpr'.
define ControlRoom as service with device.zpr.adapter.cn:'vs.zpr'.
define AttributeService as service with device.zpr.adapter.cn:'vs.zpr'.
define A1 as adapter with zpr.adapter.cn:adapter1.
define A2 as adapter with zpr.adapter.cn:adapter2.
define A3 as adapter with zpr.adapter.cn:adapter3.
define A1Svc as a service with device.zpr.adapter.cn:adapter1.
define A2Svc as a service with device.zpr.adapter.cn:adapter2.
define A3Svc as a service with device.zpr.adapter.cn:adapter3.
define Adapter2Address as a service with device.zpr.adapter.cn:adapter2.
define Adapter3Address as a service with device.zpr.adapter.cn:adapter3.

service A3Svc as json {"service_class":"A3Svc"}.
  allow A1.
  allow A2.

service A1Svc as json {"service_class":"A1Svc"}.
  allow A2.
  allow A3.

provide Adapter2Address at adapter2.svc.zpr over TCP 53.

provide Adapter3Address at adapter3.svc.zpr over TCP 53.
define AuthenticatedDevice as a device with device.zpr.adapter.cn.
define VisaDnsPublisher as an adapter with zpr.adapter.cn:'vs.zpr'.
define ZprDNS as a service with device.zpr.adapter.cn:adapter1 and zpr.addr:'fd00:1:1::1'.

provide ZprDNS at dns.svc.zpr over TCP 53.
  allow AuthenticatedDevice.
  allow VisaDnsPublisher.
define SimulatorControl as adapter with zpr.adapter.cn:'simulator-control'.
define SimulatorControlService as service with device.zpr.adapter.cn:'simulator-control' and zpr.addr:'fd5a:5052:adda:1:ffff:ffff:ffff:fffe'.
define Machine01 as a device with zpr.adapter.cn:'machine-01'.
define Machine02 as a device with zpr.adapter.cn:'machine-02'.
define Machine03 as a device with zpr.adapter.cn:'machine-03'.
define Machine04 as a device with zpr.adapter.cn:'machine-04'.
define Machine05 as a device with zpr.adapter.cn:'machine-05'.
define Machine06 as a device with zpr.adapter.cn:'machine-06'.
define Machine07 as a device with zpr.adapter.cn:'machine-07'.
define Machine08 as a device with zpr.adapter.cn:'machine-08'.
define Machine09 as a device with zpr.adapter.cn:'machine-09'.
define Machine10 as a device with zpr.adapter.cn:'machine-10'.
define Machine11 as a device with zpr.adapter.cn:'machine-11'.
define Machine12 as a device with zpr.adapter.cn:'machine-12'.
define Machine13 as a device with zpr.adapter.cn:'machine-13'.
define Machine14 as a device with zpr.adapter.cn:'machine-14'.
define Machine15 as a device with zpr.adapter.cn:'machine-15'.
define Machine16 as a device with zpr.adapter.cn:'machine-16'.
define Machine17 as a device with zpr.adapter.cn:'machine-17'.
define Machine18 as a device with zpr.adapter.cn:'machine-18'.
define Machine19 as a device with zpr.adapter.cn:'machine-19'.
define Machine20 as a device with zpr.adapter.cn:'machine-20'.

provide SimulatorControlService at simulator-control.svc.zpr over TCP 8792.
  allow Machine01.
  allow Machine02.
  allow Machine03.
  allow Machine04.
  allow Machine05.
  allow Machine06.
  allow Machine07.
  allow Machine08.
  allow Machine09.
  allow Machine10.
  allow Machine11.
  allow Machine12.
  allow Machine13.
  allow Machine14.
  allow Machine15.
  allow Machine16.
  allow Machine17.
  allow Machine18.
  allow Machine19.
  allow Machine20.
define ZprObservability as a service with device.zpr.adapter.cn:'zpr-observability' and zpr.addr:'fd5a:5052:adda:1::54'.
define TelemetryPublisher as an adapter with zpr.adapter.cn:'telemetry-publisher'.
define ObservabilityReader as an adapter with zpr.adapter.cn:'telemetry-client'.

provide ZprObservability at observability.svc.zpr over TCP 5080.
  allow TelemetryPublisher.
  allow ObservabilityReader.