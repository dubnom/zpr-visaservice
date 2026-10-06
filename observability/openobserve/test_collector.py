import unittest
import json
import tempfile
from pathlib import Path

from collector import diagnostic_log_records, organization_resource, read_diagnostic_sources


class OrganizationResourceTests(unittest.TestCase):
    def test_active_profile_is_a_resource_attribute(self):
        resource = organization_resource("load-lab")
        attributes = {
            item["key"]: item["value"]["stringValue"]
            for item in resource["attributes"]
        }

        self.assertEqual(attributes["zpr.organization.id"], "load-lab")

    def test_invalid_profile_id_is_rejected(self):
        for organization_id in ("", "Load Lab", "../northstar", "northstar/test"):
            with self.subTest(organization_id=organization_id):
                with self.assertRaises(ValueError):
                    organization_resource(organization_id)

    def test_component_telemetry_resource_identifies_source_and_organization(self):
        resource = organization_resource("redwood", "zpr-core", "north-hub", "node")
        attributes = {
            item["key"]: item["value"]["stringValue"]
            for item in resource["attributes"]
        }
        self.assertEqual(attributes["service.name"], "zpr-core")
        self.assertEqual(attributes["service.instance.id"], "north-hub")
        self.assertEqual(attributes["zpr.source.type"], "node")
        self.assertEqual(attributes["zpr.organization.id"], "redwood")

    def test_configured_source_logs_are_redacted_and_typed(self):
        source = {"source_type": "trusted-service"}
        records = diagnostic_log_records(source, ["ERROR authorization: Bearer private-token", "ready"], 123)
        self.assertEqual(len(records), 2)
        self.assertEqual(records[0]["severityText"], "ERROR")
        self.assertNotIn("private-token", records[0]["body"]["stringValue"])
        self.assertEqual(records[0]["timeUnixNano"], "123")

    def test_diagnostic_source_file_config_is_bounded_and_validated(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "sources.json"
            log_path = Path(directory) / "node.log"
            path.write_text(json.dumps({"sources": [{
                "service_name": "zpr-core", "service_instance_id": "node-a",
                "source_type": "node", "log_file": str(log_path),
                "metrics_file": str(Path(directory) / "metrics.json"),
            }]}), encoding="utf-8")
            sources = read_diagnostic_sources(path)
            self.assertEqual(sources[0]["service_name"], "zpr-core")
            path.write_text(json.dumps({"sources": [{"service_name": "bad", "service_instance_id": "../bad", "source_type": "node", "log_file": "relative.log"}]}), encoding="utf-8")
            with self.assertRaises(ValueError):
                read_diagnostic_sources(path)


if __name__ == "__main__":
    unittest.main()