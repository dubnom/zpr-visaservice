define MilwaukeeSupport as adapter with greatlakes.location:'Milwaukee, Wisconsin, USA' and greatlakes.department:'Customer Support'.
define MilwaukeeEngineering as adapter with greatlakes.location:'Milwaukee, Wisconsin, USA' and greatlakes.department:Engineering.
define MilwaukeeFinance as adapter with greatlakes.location:'Milwaukee, Wisconsin, USA' and greatlakes.department:Finance.
define MilwaukeeShipping as adapter with greatlakes.location:'Milwaukee, Wisconsin, USA' and greatlakes.department:Shipping.
define MilwaukeeReceiving as adapter with greatlakes.location:'Milwaukee, Wisconsin, USA' and greatlakes.department:Receiving.
define ShenzhenEngineering as adapter with greatlakes.location:'Shenzhen, China' and greatlakes.department:'Shenzhen Engineering'.
define TijuanaAssembly as adapter with greatlakes.location:'Tijuana, Mexico' and greatlakes.department:Assembly.
define TijuanaTest as adapter with greatlakes.location:'Tijuana, Mexico' and greatlakes.department:'Test and Quality'.
define VisaService as a service with device.zpr.adapter.cn:'vs.zpr'.

service A2Svc as json {"service_class":"A2Svc"}.
  allow A1.
  allow A3.

define CustomerSupportDesk as service with device.zpr.adapter.cn:'support-desk-app'.
define EngineeringBuildFarm as service with device.zpr.adapter.cn:'engineering-build-farm'.
define FinanceWorkspace as service with device.zpr.adapter.cn:'finance-workspace'.
define ShippingPortal as service with device.zpr.adapter.cn:'shipping-portal'.
define ReceivingPortal as service with device.zpr.adapter.cn:'receiving-portal'.
define AssemblyExecution as service with device.zpr.adapter.cn:'assembly-mes'.
define QualityTestBench as service with device.zpr.adapter.cn:'quality-test-bench'.

provide CustomerSupportDesk at customer-support.svc.zpr over TCP 8443.
  allow MilwaukeeSupport.

provide EngineeringBuildFarm at engineering-build.svc.zpr over TCP 8444.
  allow MilwaukeeEngineering.
  allow ShenzhenEngineering.

provide FinanceWorkspace at finance-workspace.svc.zpr over TCP 8445.
  allow MilwaukeeFinance.

provide ShippingPortal at shipping-portal.svc.zpr over TCP 8446.
  allow MilwaukeeShipping.

provide ReceivingPortal at receiving-portal.svc.zpr over TCP 8447.
  allow MilwaukeeReceiving.

provide AssemblyExecution at assembly-mes.svc.zpr over TCP 8448.
  allow TijuanaAssembly.

provide QualityTestBench at quality-test.svc.zpr over TCP 8449.
  allow TijuanaTest.
  allow MilwaukeeEngineering.