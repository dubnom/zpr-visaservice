#!/bin/sh
set -eu

action=${1:-}
agent=${2:-}
machine=${3:-}
rig=${SIMULATION_CONTAINER:-zpr-local-linux-node}
runtime=/run/zpr-workloads
assets=/opt/zpr-workloads
case "$agent" in
  finance-client) zpr=fd00:1:4::1; node=10.0.0.1; key=client-finance-rsa.key; tun=tun0; services='' ;;
  operations-client) zpr=fd00:1:5::1; node=10.0.0.1; key=client-operations-rsa.key; tun=tun1; services='' ;;
  telemetry-client) zpr=fd00:1:6::1; node=10.0.0.1; key=client-telemetry-rsa.key; tun=tun2; services='' ;;
  echo-service) zpr=fd00:1:7::1; node=10.0.0.1; key=service-echo-rsa.key; tun=tun3; services=EchoService ;;
  metrics-service) zpr=fd00:1:8::1; node=10.0.0.1; key=service-metrics-rsa.key; tun=tun4; services=MetricsService ;;
  internet-gateway) zpr=fd00:1:9::1; node=10.0.0.1; key=internet-gateway-rsa.key; tun=tun5; services=internet-gateway ;;
  *) echo "unknown simulated workload: $agent" >&2; exit 2 ;;
esac
case "$machine" in
  machine-[0-9][0-9]) container="zpr-$machine" ;;
  *) echo "valid machine id required" >&2; exit 2 ;;
esac

socket="$runtime/$agent.sock"
case "$action" in
  stop)
    link_id=$(docker exec "$container" "$assets/ph-cli" -p "$socket" link show 2>/dev/null | awk '/^[[:space:]]*[0-9]+:/ { sub(":", "", $1); print $1; exit }' || true)
    if [ -n "$link_id" ]; then
      docker exec "$container" "$assets/ph-cli" -p "$socket" link stop "$link_id" 2>/dev/null || true
    fi
    docker exec "$container" pkill -TERM -f "[p]h adapter.*--name $agent" 2>/dev/null || true
    docker exec "$container" rm -f "$socket" "$runtime/${agent}_cap.sock"
    ;;
  start)
    machine_ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container")
    if [ -z "$machine_ip" ] || [ -z "$node" ]; then
      echo "machine or node substrate address is unavailable" >&2
      exit 1
    fi
    link_summary=$(docker exec "$container" "$assets/ph-cli" -p "$socket" link show 2>/dev/null || true)
    case "$link_summary" in
      *"(Active)"*) echo "$agent is already connected on $machine"; exit 0 ;;
    esac
    docker exec "$container" pkill -TERM -f "[p]h adapter.*--name $agent" 2>/dev/null || true
    docker exec "$container" rm -f "$socket" "$runtime/${agent}_cap.sock"
    docker exec -d -e "ZPR_ADAPTER_SERVICES=$services" "$container" sh -c \
      'exec "$1" adapter --logging all=INFO --control-path "$2" --capture-path "$3" --self-addr "$4" --ca-file "$5" --bootstrap-key "$6" --name "$7" --km-impl noise --tun-if "$8" --node-addr "$9" --zpr-addr "${10}" >>"/tmp/$7.log" 2>&1' \
      machine-agent "$assets/ph" "$socket" "$runtime/${agent}_cap.sock" "$machine_ip" "$assets/ca.crt" "$assets/$key" "$agent" "$tun" "$node:5000" "$zpr"
    ;;
  *) echo "usage: $0 {start|stop} {finance-client|operations-client|telemetry-client|echo-service|metrics-service|internet-gateway} machine-NN" >&2; exit 2 ;;
esac
