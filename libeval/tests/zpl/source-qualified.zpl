# Source suffixes in class/group names, tags, keys, set values and scalar values.
define staff__source__corp-1_a as users with tag cleared__source__corp-1_a and user.department__source__corp-1_a:engineering__source__corp-1_a.
define client__source__lab-2_b as devices with device.role__source__lab-2_b:client__source__lab-2_b and device.groups__source__lab-2_b:{ops__source__lab-2_b, staff__source__lab-2_b}.
define database as a service with service.content__source__corp-1_a:red__source__corp-1_a.
provide database at database.svc.zpr over TCP 80.
allow staff__source__corp-1_a.
allow client__source__lab-2_b.
