package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"mealplanner/internal/households"
)

func TestHouseholdInviteCodeIsOwnerOnly(t *testing.T) {
	const bearerCode = "SECRET42"
	invite := &households.Invite{Code: bearerCode, ExpiresAt: time.Now().Add(time.Hour)}
	household := &households.Household{ID: uuid.New(), Name: "Test household"}

	render := func(role string) string {
		t.Helper()
		var out bytes.Buffer
		err := Household(HouseholdData{
			Member:    &households.Member{AccountID: uuid.New(), Name: "Viewer", Role: role},
			Household: household,
			Invite:    invite,
		}).Render(context.Background(), &out)
		if err != nil {
			t.Fatalf("render Household: %v", err)
		}
		return out.String()
	}

	memberHTML := render("member")
	if strings.Contains(memberHTML, bearerCode) || strings.Contains(memberHTML, "Invite code") {
		t.Fatalf("member page rendered invite data: %s", memberHTML)
	}

	ownerHTML := render("owner")
	if !strings.Contains(ownerHTML, bearerCode) || !strings.Contains(ownerHTML, "/household/invite/regenerate") {
		t.Fatalf("owner page did not preserve invite UX: %s", ownerHTML)
	}
}
