//go:build gointl_all

package intl_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// TestSupportedValuesMatchNode compares every list Intl.supportedValuesOf
// answers with to Node's, order included: testdata/values_node.js.
func TestSupportedValuesMatchNode(t *testing.T) {
	b, err := os.ReadFile("testdata/values_node.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if want["icu"] != "78.3" {
		t.Fatalf("the expectations come from ICU %v, not 78.3", want["icu"])
	}
	lists := map[string]func() ([]string, error){
		"calendar":        func() ([]string, error) { return intl.Calendars(intl.NodeICU), nil },
		"collation":       intl.Collations,
		"currency":        intl.Currencies,
		"numberingSystem": intl.NumberingSystems,
		"timeZone":        func() ([]string, error) { return intl.TimeZones(intl.NodeICU) },
		"unit":            func() ([]string, error) { return intl.SanctionedUnits(), nil },
	}
	for key, list := range lists {
		got, err := list()
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		var w []string
		for _, v := range want[key].([]any) {
			w = append(w, v.(string))
		}
		if strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("%s: got %d values, want %d:\n\tgot  %v\n\twant %v", key, len(got), len(w), got, w)
		}
	}
}
