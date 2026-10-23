# SYNTHETIC LAB FIXTURE ONLY. Not an approved or production Northstar policy.

define adapter as a device with zpr.adapter.cn.
define Machine01FinanceClient as adapter with zpr.adapter.cn:'finance-client'.
define NorthstarLabBaselineClient as adapter with zpr.adapter.cn:'northstar-lab-baseline-client'.
define NorthstarLabFinanceWorkspace as a service with device.zpr.adapter.cn:'finance-workspace'.

service NorthstarLabFinanceWorkspace as json {"service_class":"NorthstarLabFinanceWorkspace"}.
  # The evaluator baseline uses test-only device authority; this is not a real grant.
  allow NorthstarLabBaselineClient.
  never allow Machine01FinanceClient.
