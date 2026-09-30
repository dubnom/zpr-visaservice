#!/bin/sh
set -eu

agent=${2:-}
container=${SIMULATION_CONTAINER:-zpr-local-linux-node}
case "$agent" in
  adapter1) namespace=zpr-a; address=10.0.1.2; node=10.0.1.1; zpr=fd00:1:1::1; key=actor1-rsa.key; engine='--io-engine io_uring' ;;
  adapter2) namespace=zpr-b; address=10.0.2.2; node=10.0.2.1; zpr=fd00:1:2::1; key=actor2-rsa.key; engine='--io-engine posix_unbatched' ;;
  adapter3) namespace=zpr-c; address=10.0.3.2; node=10.0.3.129; zpr=fd00:1:3::1; key=actor3-rsa.key; engine='' ;;
  finance-client) namespace=zpr-client-a; address=10.0.4.2; node=10.0.4.1; zpr=fd00:1:4::1; key=client-finance-rsa.key; engine='' ;;
  operations-client) namespace=zpr-client-b; address=10.0.5.2; node=10.0.5.1; zpr=fd00:1:5::1; key=client-operations-rsa.key; engine='' ;;
  telemetry-client) namespace=zpr-client-c; address=10.0.6.2; node=10.0.6.1; zpr=fd00:1:6::1; key=client-telemetry-rsa.key; engine='' ;;
  echo-service) namespace=zpr-service-a; address=10.0.7.2; node=10.0.7.1; zpr=fd00:1:7::1; key=service-echo-rsa.key; engine='' ;;
  metrics-service) namespace=zpr-service-b; address=10.0.8.2; node=10.0.8.1; zpr=fd00:1:8::1; key=service-metrics-rsa.key; engine='' ;;
  *) echo "unknown simulation agent: $agent" >&2; exit 2 ;;
esac

case "$1" in
  stop)
    docker exec "$container" sh -lc "pkill -TERM -f 'ph adapter.*--name $agent' || true"
    ;;
  start)
    docker exec "$container" sh -lc "sudo -E ip netns exec $namespace sudo -E -u root /tmp/zpr-core-target/debug/ph adapter --logging all=INFO --control-path $agent.sock --capture-path ${agent}_cap.sock --self-addr $address --ca-file ca.crt --bootstrap-key $key --name $agent --km-impl noise --tun-if tun0 $engine --node-addr $node --zpr-addr $zpr >/tmp/$agent.log 2>&1 &"
    ;;
  *) echo "usage: $0 {start|stop} {adapter1|adapter2|adapter3}" >&2; exit 2 ;;
esac
