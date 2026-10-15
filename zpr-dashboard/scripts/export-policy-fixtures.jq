# Simulator provisioning owns fictional identities and inventory. Policy-Service
# receives only this versioned contract, never the source profile or manifest.
def service_attributes:
  if (.actor_cn // "") != "" then
    [{key: "device.zpr.adapter.cn", values: [.actor_cn]}]
  else [] end;

def service_export:
  . as $service |
  if ($service.policy_test_services // [] | length) > 0 then
    {services: [$service.policy_test_services[] |
      {id, name, protocol: (.protocol | ascii_upcase), port: (.port // 0),
       icmp_type: (.icmp_type // 0), icmp_code: (.icmp_code // 0),
       attributes: service_attributes}], warnings: []}
  else
    [$service.services[] |
      (.endpoint | split("/")) as $parts |
      ($parts[-2] // "" | ascii_upcase) as $protocol |
      # Match the existing endpoint adapter's leading-decimal port behavior,
      # including the first port of the Load Lab range.
      ((try ($parts[-1] | capture("^\\s*(?<port>[+-]?[0-9]+)").port | tonumber) catch 0) // 0) as $port |
      if ($parts | length) < 3 then
        {warning: "\(.name) was omitted: endpoint has no transport and port."}
      elif ($protocol != "TCP" and $protocol != "UDP") or
           $port < 1 or $port > 65535 or ($port | floor) != $port then
        {warning: "\(.name) was omitted: only TCP/UDP endpoints with valid ports can be tested."}
      else
        {service: {id: (if (.policy_id // "") != "" then .policy_id else .name end),
         name, protocol: $protocol, port: $port, attributes: service_attributes}}
      end] |
    {services: [.[] | .service // empty], warnings: [.[] | .warning // empty]}
  end;

if ($profile | length) != 1 or ($manifest | length) != 1 or
   ($profile[0].id | type) != "string" or
   ($manifest[0].machines | type) != "array" then
  error("fixture export requires one organization profile and one machine manifest")
else
  $profile[0] as $org |
  ($org.machine_owners // {}) as $owners |
  ($owners | keys) as $scope |
  [$manifest[0].machines[] |
    select(($scope | length) == 0 or (.id as $id | $scope | index($id)) != null)] as $devices |
  ($org | service_export) as $services |
  {
    version: 1, organization_id: $org.id, base_dn: $org.directory.base_dn,
    user_authority: "demo-identity",
    directory: ($org.directory |
      {attributes: (.attributes // []), user_attributes: (.user_attributes // []),
       identity_mappings: (.identity_mappings // {}),
       user_identity_mappings: (.user_identity_mappings // {})}),
    devices: [$devices[] | . as $device |
      {id, attributes: {
        "device.zpr.authority": ["zpr-bootstrap"], "device.zpr.adapter.cn": [.id],
        "device.zprMachineId": [.id], "device.zprMachineType": [.type],
        "device.zprMachineModel": [.model], "device.zprMachineLocation": [.location],
        "device.zprMachineSecure": [(.secure | tostring)],
        "device.zprMachineOwner": [($owners[$device.id][0] // .owner)]
      }}],
    device_scope: $scope,
    device_owners: (if ($scope | length) > 0 then $owners else
      [$devices[] | select((.owner // "") != "" and .owner != "it-pool") |
        {key: .id, value: [.owner]}] | from_entries end),
    user_labels: [($org.directory.people // [])[] | {key: .uid, value: .name}] | from_entries,
    services: $services.services,
    warnings: ($services.warnings + if ($scope | length) == 0 then
      ["Device ownership uses the simulation manifest because no organization owner map is configured."]
      else [] end)
  }
end
