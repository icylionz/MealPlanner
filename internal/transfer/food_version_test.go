package transfer

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFoodArchiveDoesNotCarryOptimisticLockVersion(t *testing.T) {
	var food Food
	if err := json.Unmarshal([]byte(`{"name":"Imported","version":999}`), &food); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(food)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"version"`) {
		t.Fatalf("food archive exposed a lock token: %s", encoded)
	}
}
