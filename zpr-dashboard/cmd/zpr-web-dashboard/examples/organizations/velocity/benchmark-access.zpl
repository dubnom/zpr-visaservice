define BenchmarkClient as a device with zpr.adapter.cn:finance-client.

define EchoWeb as a service with device.zpr.adapter.cn:echo-service.

provide EchoWeb at echo-web.svc.zpr over TCP 8080.
  allow BenchmarkClient.
