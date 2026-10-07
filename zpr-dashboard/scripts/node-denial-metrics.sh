#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
    echo "Usage: sh node-denial-metrics.sh CLI_PATH SOCKET_PATH SERVICE_NAME INSTANCE_ID" >&2
    exit 2
fi

if [ -n "${ZPR_NODE_CONTAINER:-}" ]; then
    counts=$(docker exec "$ZPR_NODE_CONTAINER" "$1" --socket "$2" counters)
else
    counts=$("$1" --socket "$2" counters)
fi
buffered=$(printf '%s\n' "$counts" | sed -n 's/^Buffered Denials: \([0-9][0-9]*\)$/\1/p')
local=$(printf '%s\n' "$counts" | sed -n 's/^Visa Request Backoff Denied: \([0-9][0-9]*\)$/\1/p')
case "$buffered:$local" in
    *[!0-9:]*|:*|*:) echo "Node denial counters are missing or invalid; upgrade the node runtime." >&2; exit 1 ;;
esac
timestamp="$(date +%s)000000000"
jq -cn --arg service "$3" --arg instance "$4" --arg timestamp "$timestamp" \
    --arg buffered "$buffered" --arg local "$local" --arg counts "$counts" '
    def point($value): {timeUnixNano: $timestamp, asInt: $value};
    def counter($group; $name; $value):
      {name: ("zpr.node.counters." + $group + "." + ($name | ascii_downcase | gsub("[^a-z0-9]+"; "_"))),
       unit: "{event}", sum: {aggregationTemporality: 2, isMonotonic: true, dataPoints: [point($value)]}};
    (if $counts | test("(?m)^Fastpath #[0-9]+ counts:$") then
      reduce ($counts | split("\n")[]) as $line
        ({group: null, metrics: []};
         if $line == "Management counts:" then .group = "management"
         elif $line | test("^Fastpath #[0-9]+ counts:$") then
           .group = ("fastpath." + ($line | capture("^Fastpath #(?<id>[0-9]+) counts:$").id))
         elif $line == "" or ($line | test("^Uptime: [0-9]+\\.[0-9]+$")) then .
         elif .group != null and ($line | test("^[A-Za-z][A-Za-z0-9 -]*: [0-9]+$")) then
           ($line | capture("^(?<name>[^:]+): (?<value>[0-9]+)$")) as $entry |
           if .group == "management" and $entry.name == "Buffered Denials" then .
           else .metrics += [counter(.group; $entry.name; $entry.value)] end
         else error("Invalid node counter output") end) | .metrics
    else [] end) as $counters |
    {resourceMetrics: [{
      resource: {attributes: [
        {key: "service.name", value: {stringValue: $service}},
        {key: "service.instance.id", value: {stringValue: $instance}}
      ]},
      scopeMetrics: [{scope: {name: "zpr.node.denials"}, metrics: ([
        {name: "zpr.node.denials.buffered", unit: "{flow}", gauge: {dataPoints: [{timeUnixNano: $timestamp, asInt: $buffered}]}},
        {name: "zpr.node.denials.local", unit: "{request}", sum: {aggregationTemporality: 2, isMonotonic: true, dataPoints: [{timeUnixNano: $timestamp, asInt: $local}]}}
      ] + $counters)}]
    }]}'
