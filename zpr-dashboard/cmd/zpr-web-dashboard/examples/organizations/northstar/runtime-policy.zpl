define DemoVerified as adapter with demo.verified:yes.
define DemoEngineer as adapter with demo.department:Engineering.

service A2Svc as json {"service_class":"A2Svc"}.
  allow DemoVerified.
  allow DemoEngineer.
  allow A1.
  allow A3.
define FinanceClient as adapter with zpr.adapter.cn:'finance-client'.
define OperationsClient as adapter with zpr.adapter.cn:'operations-client'.
define TelemetryClient as adapter with zpr.adapter.cn:'telemetry-client'.
define EchoService as service with device.zpr.adapter.cn:'echo-service'.
define EchoWeb as service with device.zpr.adapter.cn:'echo-service'.
define MetricsService as service with device.zpr.adapter.cn:'metrics-service'.
define MetricsWeb as service with device.zpr.adapter.cn:'metrics-service'.
define gateway as a service with external-network-connection.
define internet-gateway as a gateway with external-network-connection:public-internet.
define InternetGatewayWeb as an internet-gateway with device.zpr.adapter.cn:'internet-gateway'.

provide EchoWeb at echo-web.svc.zpr over TCP 8080.
  allow FinanceClient.

provide MetricsWeb at metrics-web.svc.zpr over TCP 8081.
  allow OperationsClient.

provide InternetGatewayWeb at internet-gateway.svc.zpr over TCP 8082.
  allow FinanceClient.