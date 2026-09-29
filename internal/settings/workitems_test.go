package settings

import (
	"path/filepath"
	"testing"
)

func TestValidateWorkItemURL(t *testing.T) {
	for _, tc := range []struct {
		template string
		ok       bool
	}{
		{"", true},
		{"https://dev.azure.com/barreleye/Barreleye/_workitems/edit/{id}", true},
		{"http://tracker.local/issues/{id}?view=full", true},
		{"https://dev.azure.com/barreleye/Barreleye/_workitems/edit/", false}, // nowhere for the number
		{"dev.azure.com/org/{id}", false},                                     // no scheme
		{"javascript:alert({id})", false},                                     // not a web link
		{"https:///{id}", false},                                              // no host
	} {
		if err := ValidateWorkItemURL(tc.template); (err == nil) != tc.ok {
			t.Errorf("ValidateWorkItemURL(%q) = %v, want ok=%v", tc.template, err, tc.ok)
		}
	}
}

func TestSaveRejectsBadWorkItemURL(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{WorkItemURL: "not a url {id}"}); err == nil {
		t.Error("saved an invalid work item URL")
	}
	good := Settings{WorkItemURL: "https://example.com/items/{id}"}
	if err := store.Save(good); err != nil {
		t.Fatal(err)
	}
	loaded, err := Open(store.path).Load()
	if err != nil || loaded.WorkItemURL != good.WorkItemURL {
		t.Errorf("round trip = %+v, %v", loaded, err)
	}
}
