#!/bin/sh
set -eu
# Private fixture extension for probe-node-image.cjs, not installation setup.
[ "$#" -eq 0 ] || { echo "prepare-peer-probe: no arguments expected" >&2; exit 2; }
[ -f /state/first/visa-bootstrap.json ] || { echo "prepare-peer-probe: fresh node state required" >&2; exit 1; }
umask 077
mkdir /state/peer
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
    -out /state/peer/node.key > /state/peer/key-generation.log 2>&1
openssl pkey -in /state/peer/node.key -pubout -out /state/peer/node.pub.pem
openssl genpkey -algorithm X25519 -out /state/peer/node-noise.key >> /state/peer/key-generation.log 2>&1
openssl pkey -in /state/peer/node-noise.key -pubout -out /state/peer/node-noise.pub.pem
openssl x509 -new -force_pubkey /state/peer/node-noise.pub.pem \
    -subj /CN=peer -CA /state/first/authority.pem -CAkey /state/first/authority.key \
    -CAserial /state/first/authority.srl -days 90 -sha256 -out /state/peer/node-noise.pem \
    >> /state/peer/key-generation.log 2>&1
openssl verify -CAfile /state/first/authority.pem /state/peer/node-noise.pem > /state/peer/certificate-verification.txt
cp /state/first/authority.pem /state/peer/authority.pem
cp /state/first/node-noise.pem /state/peer/remote-noise.pem
cp /state/peer/node-noise.pem /state/first/remote-noise.pem
cp /state/second/node-noise.pem /state/first/wrong-noise.pem
cat >> /state/first/node.toml <<'EOF'

[node.peer_noise_certificates]
"fd5a:5052::3" = "/state/first/remote-noise.pem"

[[node.peers]]
zpr_addr = "fd5a:5052::3"
substrate_addr = "[fd00:5a::5]:5000"
link_id = "node-peer"
EOF
cat > /state/peer/node.toml <<'EOF'
[global]
name = "peer"
ca_file = "/state/peer/authority.pem"
certificate_file = "/state/peer/node-noise.pem"
private_key_file = "/state/peer/node-noise.key"
self_addr = "[fd00:5a::5]:5000"
zpr_addr = ["fd5a:5052::3"]
tun_if = "zprtest0"
control_path = "/state/peer/node-control.sock"
capture_path = "/state/peer/node-capture.sock"

[authentication]
auth_private_key = "/state/peer/node.key"

[node]
advertised_substrate_addr = "[fd00:5a::5]:5000"
local_vs_dock = true

[node.peer_noise_certificates]
"fd5a:5052::2" = "/state/peer/remote-noise.pem"

[[node.peers]]
zpr_addr = "fd5a:5052::2"
substrate_addr = "[fd00:5a::2]:5000"
link_id = "node-peer"
EOF
sed '/^"node" = /a\
"peer" = "/state/peer/node.pub.pem"
' /state/first/bootstrap.zplc > /state/peer/policy.zplc
cat >> /state/peer/policy.zplc <<'EOF'

[nodes.peer]
provider = [["device.zpr.adapter.cn", "peer"]]
zpr_address = "fd5a:5052::3"

[nodes.node.substrate_addrs]
probe = "[fd00:5a::2]:5000"

[nodes.peer.substrate_addrs]
probe = "[fd00:5a::5]:5000"

[links.node-peer]
peers = [{node = "node"}, {node = "peer"}]

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
cat > /state/peer/policy.zpl <<'EOF'
define Node as a device with zpr.adapter.cn:node.
define Peer as a device with zpr.adapter.cn:peer.
define PingableVs as a service with device.zpr.adapter.cn:'vs.zpr'.
service PingableVs as json {"service_class":"PingableVs"}.
  allow Node.
  allow Peer.
define AllowedProbe as a service with device.zpr.adapter.cn:peer.
service AllowedProbe as json {"service_class":"AllowedProbe"}.
  allow Node.
define DeniedProbe as a service with device.zpr.adapter.cn:peer.
service DeniedProbe as json {"service_class":"DeniedProbe"}.
  allow Peer.
EOF
zplc /state/peer/policy.zpl -c /state/peer/policy.zplc \
    -k /state/first/policy-signing.key -d /state/peer -o bootstrap.bin2 \
    > /state/peer/compiler.log 2>&1
test -s /state/peer/bootstrap.bin2
