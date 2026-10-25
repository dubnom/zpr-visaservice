package controlroom

import "testing"

func TestSubjectLinkRegistryPermissionMapping(t *testing.T) {
	for _, test := range []struct{ method, path, permission string }{
		{"PUT", "/api/subject-link-registry/authorities/bas", "subject-link.registry.manage"},
		{"PUT", "/api/subject-link-registry/sources/company", "subject-link.registry.manage"},
		{"GET", "/api/subject-link-registry/sources/company", "subject-link.read"},
		{"GET", "/api/subject-link-registry/authorities/bas/versions/2", "subject-link.read"},
		{"POST", "/api/subject-link-registry/context", "subject-link.read"},
		{"DELETE", "/api/subject-link-registry/sources/company", ""},
		{"HEAD", "/api/subject-link-registry/sources/company", ""},
		{"GET", "/api/subject-link-registry/sources/company/versions/01", ""},
		{"POST", "/api/subject-link-registry/sources/company", ""},
		{"PUT", "/api/subject-link-registry/sources/company/extra", ""},
		{"GET", "/api/subject-link-registry/authorities/%62as", ""},
	} {
		permission, mapped := APIPermission(test.method, test.path)
		if permission != test.permission || mapped != (test.permission != "") {
			t.Fatalf("%s %s: permission=%q mapped=%v", test.method, test.path, permission, mapped)
		}
	}
}
