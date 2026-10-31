package enrollment

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestEnrollmentMaximumBatchFitsSignedDelegationBodyBound(t *testing.T) {
	request := BatchApprovalRequest{Organization: strings.Repeat("\x01", 256),
		InventoryDigest: strings.Repeat("a", 64), Reason: strings.Repeat("\x02", 256)}
	for index := range 128 {
		request.InvitationIDs = append(request.InvitationIDs, fmt.Sprintf("%026d", index))
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > operatordelegation.MaxBody {
		t.Fatalf("128 invitations exceed the unchanged signed delegation body bound: %d > %d", len(data), operatordelegation.MaxBody)
	}
}
