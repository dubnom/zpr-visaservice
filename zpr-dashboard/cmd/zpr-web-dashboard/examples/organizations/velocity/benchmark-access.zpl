define BenchmarkClient as a device with zpr.adapter.cn:finance-client.
define EchoWeb as a service with device.zpr.adapter.cn:echo-service.
allow BenchmarkClient to access EchoWeb.