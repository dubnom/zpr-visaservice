import unittest

from collector import organization_resource


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


if __name__ == "__main__":
    unittest.main()