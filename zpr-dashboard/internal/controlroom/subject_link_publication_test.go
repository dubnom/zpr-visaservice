package controlroom

import "testing"

func TestSubjectLinkPublicationPermissionMapping(t *testing.T) {
	const prefix = "/api/subject-link-publications/vs-1/epoch-1"
	for _, test := range []struct{ method, path, permission string }{
		{"GET", prefix, "subject-link.read"},
		{"GET", prefix + "/events/0", "subject-link.read"},
		{"GET", prefix + "/candidates/2", "subject-link.read"},
		{"GET", prefix + "/signed/2", "subject-link.read"},
		{"GET", prefix + "/receipts/2", "subject-link.read"},
		{"POST", prefix + "/provision", "subject-link.publish"},
		{"POST", prefix + "/intents", "subject-link.publish"},
		{"POST", prefix + "/candidates", "subject-link.snapshot.capture"},
		{"POST", prefix + "/candidates/2/sign", "subject-link.snapshot.sign"},
		{"HEAD", prefix, ""},
		{"PUT", prefix + "/provision", ""},
		{"DELETE", prefix + "/signed/2", ""},
		{"GET", prefix + "/candidates/0", ""},
		{"GET", prefix + "/signed/02", ""},
		{"POST", prefix + "/candidates/2/sign/extra", ""},
		{"POST", prefix + "/signed/2", ""},
		{"GET", prefix + "/receipts/0", ""},
		{"POST", prefix + "/receipts", ""},
		{"GET", prefix + "/receipts", ""},
		{"GET", "/api/subject-link-publications/vs-1", ""},
		{"GET", "/api/subject-link-publications/vs.1/epoch-1", ""},
		{"POST", prefix + "/activate", ""},
	} {
		permission, mapped := APIPermission(test.method, test.path)
		if permission != test.permission || mapped != (test.permission != "") {
			t.Fatalf("%s %s: permission=%q mapped=%v", test.method, test.path, permission, mapped)
		}
	}
}
