define database as a service.
define employee as a user with user.bas_id.
define signalService as a service.
define SalesDatabase as a database with device.tint:sales.

# No ON

service database as json {"service_class":"database"}.
  allow color:red employees and signal "red employee" to signalService.
  allow employees and signal "employee" to signalService.
  allow employees on hardened devices and signal "accessed" to signalService.

# Provider-device conditions are part of the service class.

service SalesDatabase as json {"service_class":"SalesDatabase"}.
  allow color:red employees and signal "red tint access" to signalService.
