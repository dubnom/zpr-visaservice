# Simulator provisioning only. CNs match prepare_organization_nodes in deploy-docker.sh.
def valid_coordinates:
  if .latitude == null and .longitude == null then true
  elif (.latitude | type) != "number" or (.longitude | type) != "number" then false
  else .latitude >= -90 and .latitude <= 90 and .longitude >= -180 and .longitude <= 180
  end;

.runtime.nodes
| if type != "array" then error("runtime.nodes must be an array")
  elif all(.[]; valid_coordinates) then .
  else error("node coordinates require a numeric latitude/longitude pair within [-90,90]/[-180,180]")
  end
| to_entries[]
| select(.value.latitude != null)
| "\n# Simulator display coordinates; may be approximate guesses. See organization coordinate_note.\n[nodes.\"node\(.key).demo\"]\nlatitude = \(.value.latitude)\nlongitude = \(.value.longitude)"
