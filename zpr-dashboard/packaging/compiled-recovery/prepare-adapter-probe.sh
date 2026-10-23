#!/bin/sh
set -eu
# Private fixture extension for probe-node-image.cjs, not installation setup.
[ "$#" -eq 0 ] || { echo "prepare-adapter-probe: no arguments expected" >&2; exit 2; }
[ -f /state/first/visa-bootstrap.json ] || { echo "prepare-adapter-probe: fresh node state required" >&2; exit 1; }
umask 077
mkdir /state/adapter
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
    -out /state/adapter/bootstrap.key > /state/adapter/key-generation.log 2>&1
openssl pkey -in /state/adapter/bootstrap.key -pubout -out /state/adapter/bootstrap.pub.pem
openssl genpkey -algorithm X25519 -out /state/adapter/noise.key >> /state/adapter/key-generation.log 2>&1
openssl pkey -in /state/adapter/noise.key -pubout -out /state/adapter/noise.pub.pem
openssl x509 -new -force_pubkey /state/adapter/noise.pub.pem \
    -subj /CN=client -CA /state/first/authority.pem -CAkey /state/first/authority.key \
    -CAserial /state/first/authority.srl -days 90 -sha256 -out /state/adapter/noise.pem \
    >> /state/adapter/key-generation.log 2>&1
cp /state/first/authority.pem /state/adapter/authority.pem
cat > /state/adapter/config.toml <<'EOF'
[global]
name = "client"
ca_file = "/state/adapter/authority.pem"
certificate_file = "/state/adapter/noise.pem"
private_key_file = "/state/adapter/noise.key"
self_addr = "[fd00:5a::4]:5000"
tun_if = "zprclient0"
control_path = "/state/adapter/control.sock"
capture_path = "/state/adapter/capture.sock"

[adapter]
node_addr = "[fd00:5a::2]:5000"
bootstrap_key = "/state/adapter/bootstrap.key"
EOF
sed '/^"node" = /a\
"client" = "/state/adapter/bootstrap.pub.pem"
' /state/first/bootstrap.zplc > /state/adapter/policy.zplc
cat >> /state/adapter/policy.zplc <<'EOF'

[protocols.allowed_probe]
l4protocol = "iana.TCP"
port = 16379

[protocols.denied_probe]
l4protocol = "iana.TCP"
port = 16380

[services.AllowedProbe]
protocol = "allowed_probe"

[services.DeniedProbe]
protocol = "denied_probe"
EOF
cat > /state/adapter/policy.zpl <<'EOF'
define Node as a device with zpr.adapter.cn:node.
define Client as a device with zpr.adapter.cn:client.
define Vs as a device with zpr.adapter.cn:'vs.zpr'.
define PingableVs as a service with device.zpr.adapter.cn:'vs.zpr'.
service PingableVs as json {"service_class":"PingableVs"}.
  allow Node.
  allow Client.
define AllowedProbe as a service with device.zpr.adapter.cn:node.
service AllowedProbe as json {"service_class":"AllowedProbe"}.
  allow Client.
define DeniedProbe as a service with device.zpr.adapter.cn:node.
service DeniedProbe as json {"service_class":"DeniedProbe"}.
  allow Node.
EOF
zplc /state/adapter/policy.zpl -c /state/adapter/policy.zplc \
    -k /state/first/policy-signing.key -d /state/adapter -o bootstrap.bin2 \
    > /state/adapter/compiler.log 2>&1
test -s /state/adapter/bootstrap.bin2
