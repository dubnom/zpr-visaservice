# The DNS actor joins with the configured service-provider identity.
define ZprDNS as a service with device.zpr.adapter.cn:zpr-dns and zpr.addr:'fd5a:5052:adda:1::53'.

define ZprDNSStatistics as a service with device.zpr.adapter.cn:zpr-dns and zpr.addr:'fd5a:5052:adda:1::53'.

# A successfully authenticated ZPR device receives device.zpr.adapter.cn.
define AuthenticatedDevice as a device with device.zpr.adapter.cn.
define VisaDnsPublisher as an adapter with zpr.adapter.cn:'vs.zpr'.

provide ZprDNS at dns.svc.zpr over TCP 53.
allow AuthenticatedDevice.
allow VisaDnsPublisher.

provide ZprDNSStatistics at dns-stats.svc.zpr over TCP 8053.
allow VisaDnsPublisher.
