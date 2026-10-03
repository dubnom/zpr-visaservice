define database as a service.
define employee as a user with user.bas_id.
define signalService as a service.
define pingy as a service.
define web1 as a service.
define Webby as a service.
define SalesDatabase as a database with device.tint:sales.

service database as json {"service_class":"database"}.
  allow color:red employees and signal "red employee" to signalService.
  allow employees and signal "employee" to signalService.
  allow employees on hardened devices and signal "accessed" to signalService.

service SalesDatabase as json {"service_class":"SalesDatabase"}.
  allow color:red employees and signal "red tint access" to signalService.

service pingy as json {"service_class":"pingy"}.
  allow color:red employees.

service web1 as json {"service_class":"web1"}.
  allow color:red employees on hardened devices.

service Webby as json {"service_class":"Webby"}.
  allow users.
