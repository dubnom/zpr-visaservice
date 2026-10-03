define database as a service.
define employee as a user with user.bas_id.
define signalService as a service.
define Monitor as a service.
define SalesDatabase as a database with device.tint:sales.

service Monitor as json {"service_class":"Monitor"}.
  never allow employees.

service database as json {"service_class":"database"}.
  allow color:red employees and signal "red employee" to signalService.
  allow employees and signal "employee" to signalService.
  allow employees on hardened devices and signal "accessed" to signalService.

service SalesDatabase as json {"service_class":"SalesDatabase"}.
  allow color:red employees and signal "red tint access" to signalService.
