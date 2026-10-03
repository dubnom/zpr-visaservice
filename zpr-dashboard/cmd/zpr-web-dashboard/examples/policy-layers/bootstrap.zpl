define adapter as a device with zpr.adapter.cn.
define Node as adapter with zpr.adapter.cn:node.
define Vs as adapter with zpr.adapter.cn:'vs.zpr'.
define PingableVs as a service with device.zpr.adapter.cn:'vs.zpr'.
define PingableNode as a service with device.zpr.adapter.cn:node.

service PingableVs as json {"service_class":"PingableVs"}.
  allow Node.

service PingableNode as json {"service_class":"PingableNode"}.
  allow Vs.
define VsAdmin as a device with zpr.adapter.cn:'client.zpr.org'.

provide VisaService at visa-admin.svc.zpr over TCP 443.
  allow VsAdmin.