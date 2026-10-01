# The DNS actor joins with the configured service-provider identity.
define ZprDNS as a service with device.zpr.adapter.cn:zpr-dns and zpr.addr:'fd5a:5052:adda:1::53'.
provide ZprDNS at dns.svc.zpr over TCP 53.

define ZprDNSStatistics as a service with device.zpr.adapter.cn:zpr-dns and zpr.addr:'fd5a:5052:adda:1::53'.
provide ZprDNSStatistics at dns-stats.svc.zpr over TCP 8053.

# A successfully authenticated ZPR device receives device.zpr.adapter.cn.
define AuthenticatedDevice as a device with device.zpr.adapter.cn.
allow AuthenticatedDevices to access ZprDNS.
define VisaDnsPublisher as a device with device.zpr.adapter.cn:'vs.zpr'.
allow VisaDnsPublisher to access ZprDNS.
allow VisaDnsPublisher to access ZprDNSStatistics.
