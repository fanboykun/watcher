package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/fanboykun/watcher/internal/database"
)

func TestWatcherCandidateListsSavedTargetsAndDistinguishesFallbackDraft(t *testing.T) {
	router, db, watcher, service := candidateAPIFixture(t)
	other := database.Watcher{Name: "other pool"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherService := database.Service{WatcherID: other.ID}
	if err := db.Create(&otherService).Error; err != nil {
		t.Fatal(err)
	}
	for _, revision := range []database.ServiceConfigRevision{
		{ServiceID: service.ID, TargetVersion: "next", EnvContent: "NEXT=true"},
		{ServiceID: service.ID, TargetVersion: "v2", EnvContent: "EXACT=true"},
		{ServiceID: service.ID, TargetVersion: "v3", EnvContent: "FUTURE=true"},
		{ServiceID: otherService.ID, TargetVersion: "other-pool-only", EnvContent: "PRIVATE=true"},
	} {
		if err := db.Create(&revision).Error; err != nil {
			t.Fatal(err)
		}
	}
	load := func(target string) WatcherCandidateResponse {
		t.Helper()
		rec := authRequest(router, http.MethodGet, fmt.Sprintf("/api/watchers/%d/candidate?target=%s", watcher.ID, target), "", "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("load target: %d %s", rec.Code, rec.Body.String())
		}
		var response WatcherCandidateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	exact := load("v2")
	if !reflect.DeepEqual(exact.SavedTargets, []string{"next", "v2", "v3"}) || len(exact.Services) != 1 || exact.Services[0].CandidateTarget != "v2" || exact.Services[0].CandidateEnv != "EXACT=true" {
		t.Fatalf("saved-target discovery or exact draft identity: %+v", exact)
	}
	rec := authRequest(router, http.MethodDelete, fmt.Sprintf("/api/services/%d/revisions/v2", service.ID), "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete selected draft: %d %s", rec.Code, rec.Body.String())
	}
	fallback := load("v2")
	if !reflect.DeepEqual(fallback.SavedTargets, []string{"next", "v3"}) || fallback.Services[0].CandidateTarget != "next" || fallback.Services[0].CandidateEnv != "NEXT=true" {
		t.Fatalf("deleted exact target was confused with the next draft: %+v", fallback)
	}
}
