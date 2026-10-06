define database as a service.
define pingdb as a service.
define RedDatabase as a database with service.content:red.
define RedPingDb as a pingdb with service.content:red.

service database as json {"service_class":"database"}.
  never allow green users.

service RedDatabase as json {"service_class":"RedDatabase"}.
  allow red users.
  never allow green users.

service pingdb as json {"service_class":"pingdb"}.
  allow red users.
  never allow green users.

service RedPingDb as json {"service_class":"RedPingDb"}.
  allow red users.
  never allow green users.
