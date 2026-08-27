package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"mealplanner/internal/foods"
	"mealplanner/internal/households"
)

func TestFoodDetailDoesNotLinkUnsafeSourceURL(t *testing.T) {
	d := FoodDetailData{
		Member: &households.Member{Name: "Owner", Role: "owner", Initials: "OW"},
		Food: foods.Food{
			ID: uuid.New(), Name: "Soup", SourceURL: "javascript:alert(1)",
		},
	}
	var out bytes.Buffer
	if err := FoodDetail(d).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if !strings.Contains(html, "javascript:alert(1)") {
		t.Fatal("unsafe source URL was not shown as text")
	}
	if strings.Contains(html, `href="javascript:alert(1)"`) || strings.Contains(html, ">Re-import</button>") {
		t.Fatal("unsafe source URL was rendered as an action")
	}
}
