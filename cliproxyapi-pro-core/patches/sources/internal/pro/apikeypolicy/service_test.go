package apikeypolicy

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "pro.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetTakeover(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestTakeoverControlsNewRequestDecisionsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "takeover.sqlite")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	identity := testIdentity(t, "takeover-key")
	if _, err = service.Create(context.Background(), identity, "Takeover", ProfileInput{
		Name: "Restricted", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	}); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Mode != ModePassthrough || decision.Snapshot != nil {
		t.Fatalf("disabled decision = %#v, %v", decision, err)
	}
	if err = service.SetTakeover(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.Decide(identity)
	if err != nil || frozen.Mode != ModeProfile || frozen.Snapshot == nil {
		t.Fatalf("enabled decision = %#v, %v", frozen, err)
	}
	if err = service.SetTakeover(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	decision, err = service.Decide(identity)
	if err != nil || decision.Mode != ModePassthrough || frozen.Mode != ModeProfile {
		t.Fatalf("disabled/frozen decisions = %#v / %#v, %v", decision, frozen, err)
	}
	if err = service.SetTakeover(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewService(reopenedStore)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.TakeoverEnabled() {
		t.Fatal("takeover setting did not persist")
	}
}

func TestUnhealthyPolicyServiceCanPersistEmergencyTakeoverStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "takeover-emergency-stop.sqlite")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	identity := testIdentity(t, "emergency-stop-key")
	if _, err = service.Create(context.Background(), identity, "Emergency", ProfileInput{
		Name: "Restricted", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = service.SetTakeover(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	service.MarkUnavailable()
	if service.Healthy() || !service.TakeoverEnabled() {
		t.Fatalf("unavailable state healthy=%v takeover=%v", service.Healthy(), service.TakeoverEnabled())
	}
	if err = service.SetTakeoverIfGeneration(context.Background(), true, service.PolicyGeneration(), service.ConfiguredGeneration()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unhealthy activation error=%v", err)
	}
	if err = service.SetTakeover(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if service.Healthy() || service.TakeoverEnabled() {
		t.Fatalf("emergency stop state healthy=%v takeover=%v", service.Healthy(), service.TakeoverEnabled())
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Mode != ModePassthrough {
		t.Fatalf("emergency stop decision=%#v err=%v", decision, err)
	}
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewService(reopenedStore)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.TakeoverEnabled() {
		t.Fatal("emergency stop did not persist")
	}
}

func TestTakeoverActivationRequiresConfirmedPolicyAndConfiguredGenerations(t *testing.T) {
	service := newTestService(t)
	if err := service.SetTakeover(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	service.SetConfiguredAPIKeys([]string{"generation-key"})
	policyGeneration := service.PolicyGeneration()
	configuredGeneration := service.ConfiguredGeneration()
	identity := testIdentity(t, "generation-key")
	if _, err := service.Create(context.Background(), identity, "Generation", ProfileInput{
		Name: "Restricted", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetTakeoverIfGeneration(context.Background(), true, policyGeneration, configuredGeneration); !errors.Is(err, ErrTakeoverStateChanged) {
		t.Fatalf("stale policy generation error=%v", err)
	}
	if service.TakeoverEnabled() {
		t.Fatal("stale policy generation enabled takeover")
	}
	policyGeneration = service.PolicyGeneration()
	service.SetConfiguredAPIKeys([]string{"generation-key", "new-key"})
	if err := service.SetTakeoverIfGeneration(context.Background(), true, policyGeneration, configuredGeneration); !errors.Is(err, ErrTakeoverStateChanged) {
		t.Fatalf("stale configured generation error=%v", err)
	}
	if service.TakeoverEnabled() {
		t.Fatal("stale configured generation enabled takeover")
	}
	if err := service.SetTakeoverIfGeneration(context.Background(), true, service.PolicyGeneration(), service.ConfiguredGeneration()); err != nil {
		t.Fatal(err)
	}
}

func testIdentity(t *testing.T, raw string) AuthenticatedAPIKeyIdentity {
	t.Helper()
	identity, err := NewAuthenticatedAPIKeyIdentity(raw)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestFingerprintGoldenAndContextCannotUseClientFields(t *testing.T) {
	identity := testIdentity(t, "raw-api-key")
	if got, want := identity.Hash(), "7394b42f2b2f119e4d91b02296fc193cd2bd0da8f13419af3499dd01a4eb6734"; got != want {
		t.Fatalf("hash = %q, want %q", got, want)
	}
	ctx := WithIdentity(context.Background(), identity)
	stored, ok := IdentityFromContext(ctx)
	if !ok || stored.Hash() != identity.Hash() {
		t.Fatalf("context identity = %#v, %v", stored, ok)
	}
	if _, ok := IdentityFromContext(context.Background()); ok {
		t.Fatal("empty context forged an API key identity")
	}
}

func TestCreatePolicyPublishesImmutableActiveProfile(t *testing.T) {
	service := newTestService(t)
	identity := testIdentity(t, "key-a")
	policy, err := service.Create(context.Background(), identity, "Key A", ProfileInput{
		Name: "default", Providers: []string{"Codex", "claude"}, Models: []string{"gpt-5", "claude-opus"},
		Mappings: []ModelMapping{{Source: "smart", Target: "gpt-5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.ActiveProfileID == "" || len(policy.Profiles) != 1 || policy.ActiveProfileID != policy.Profiles[0].ID {
		t.Fatalf("policy = %#v", policy)
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Mode != ModeProfile {
		t.Fatalf("decision = %#v, %v", decision, err)
	}
	effective, err := decision.ApplyModel("smart")
	if err != nil || effective != "gpt-5" {
		t.Fatalf("effective model = %q, %v", effective, err)
	}
	providers, err := decision.FilterProviders([]string{"gemini", "codex", "claude"})
	if err != nil || len(providers) != 2 || providers[0] != "codex" || providers[1] != "claude" {
		t.Fatalf("providers = %#v, %v", providers, err)
	}

	old := decision.Clone()
	created, err := service.CreateProfile(context.Background(), policy.ID, policy.Version, ProfileInput{
		Name: "restricted", Providers: []string{"gemini"}, Models: []string{"gemini-2.5-pro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var restrictedProfileID string
	for _, profile := range created.Profiles {
		if profile.ID != policy.ActiveProfileID {
			restrictedProfileID = profile.ID
			break
		}
	}
	if restrictedProfileID == "" {
		t.Fatalf("created policy has no inactive profile: %#v", created)
	}
	updated, err := service.ActivateProfile(context.Background(), policy.ID, restrictedProfileID, created.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ActiveProfileID == policy.ActiveProfileID {
		t.Fatal("active profile did not switch")
	}
	if effective, err := old.ApplyModel("smart"); err != nil || effective != "gpt-5" {
		t.Fatalf("in-flight snapshot changed: %q, %v", effective, err)
	}
	newDecision, err := service.Decide(identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newDecision.ApplyModel("smart"); err == nil {
		t.Fatal("new active profile allowed an old mapping")
	}
}

func TestPolicyJSONUsesEmptyCollectionsInsteadOfNull(t *testing.T) {
	service := newTestService(t)
	policy, err := service.Create(context.Background(), testIdentity(t, "empty-mappings-key"), "Empty mappings", ProfileInput{
		Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"mappings":null`) || !strings.Contains(string(payload), `"mappings":[]`) {
		t.Fatalf("policy JSON must use an empty mappings array: %s", payload)
	}
}

func TestProfileCatalogReturnsOnlyCurrentNamesWithAtomicGeneration(t *testing.T) {
	service := newTestService(t)
	policy, err := service.Create(context.Background(), testIdentity(t, "profile-catalog-key"), "Catalog", ProfileInput{
		Name: "Original", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ListProfileCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != policy.ActiveProfileID || first.Items[0].Name != "Original" || first.PolicyGeneration != service.PolicyGeneration() {
		t.Fatalf("first catalog = %#v", first)
	}
	edited, err := service.ReplaceProfile(context.Background(), policy.ID, policy.ActiveProfileID, policy.Version, ProfileInput{
		Name: "Current", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ListProfileCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Name != "Current" || second.Items[0].UpdatedAtMS != edited.Profiles[0].UpdatedAtMS || second.PolicyGeneration <= first.PolicyGeneration {
		t.Fatalf("second catalog = %#v; first generation = %d", second, first.PolicyGeneration)
	}
	service.MarkUnavailable()
	if _, err := service.ListProfileCatalog(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unhealthy profile catalog error = %v", err)
	}
}

func TestInFlightSnapshotSurvivesProfileEditActivationAndPolicyDelete(t *testing.T) {
	service := newTestService(t)
	identity := testIdentity(t, "in-flight-key")
	policy, err := service.Create(context.Background(), identity, "In flight", ProfileInput{
		Name: "first", Providers: []string{"codex"}, Models: []string{"gpt-5"},
		Mappings: []ModelMapping{{Source: "smart", Target: "gpt-5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := service.Decide(identity)
	if err != nil {
		t.Fatal(err)
	}
	frozen = frozen.WithModels("smart", "gpt-5")
	assertFrozen := func(stage string) {
		t.Helper()
		if frozen.Snapshot == nil || frozen.Snapshot.PolicyID != policy.ID || frozen.Snapshot.ProfileID != policy.ActiveProfileID || frozen.Snapshot.ProfileName != "first" || frozen.Snapshot.Version != policy.Version {
			t.Fatalf("%s changed frozen identity: %#v", stage, frozen)
		}
		if got, applyErr := frozen.ApplyModel("smart"); applyErr != nil || got != "gpt-5" {
			t.Fatalf("%s changed frozen mapping: %q, %v", stage, got, applyErr)
		}
		if attribution := frozen.UsageAttribution(); attribution.APIKeyPolicyID != policy.ID || attribution.ProfileID != policy.ActiveProfileID || attribution.ProfileName != "first" || attribution.RequestedModel != "smart" || attribution.EffectiveModel != "gpt-5" {
			t.Fatalf("%s changed frozen usage attribution: %#v", stage, attribution)
		}
	}
	assertFrozen("initial")

	edited, err := service.ReplaceProfile(context.Background(), policy.ID, policy.ActiveProfileID, policy.Version, ProfileInput{
		Name: "first-edited", Providers: []string{"claude"}, Models: []string{"claude-sonnet-4-6"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFrozen("profile edit")
	current, err := service.Decide(identity)
	if err != nil || current.Snapshot == nil || current.Snapshot.ProfileName != "first-edited" {
		t.Fatalf("new decision after edit = %#v, %v", current, err)
	}

	withSecond, err := service.CreateProfile(context.Background(), policy.ID, edited.Version, ProfileInput{
		Name: "second", Providers: []string{"gemini"}, Models: []string{"gemini-2.5-pro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID := ""
	for _, profile := range withSecond.Profiles {
		if profile.Name == "second" {
			secondID = profile.ID
		}
	}
	activated, err := service.ActivateProfile(context.Background(), policy.ID, secondID, withSecond.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertFrozen("active switch")
	current, err = service.Decide(identity)
	if err != nil || current.Snapshot == nil || current.Snapshot.ProfileID != secondID {
		t.Fatalf("new decision after activation = %#v, %v", current, err)
	}

	if err := service.DeletePolicy(context.Background(), policy.ID, activated.Version, PassthroughConfirmation); err != nil {
		t.Fatal(err)
	}
	assertFrozen("policy delete")
	current, err = service.Decide(identity)
	if err != nil || current.Mode != ModePassthrough || current.Snapshot != nil {
		t.Fatalf("new decision after delete = %#v, %v", current, err)
	}
	audits, err := service.store.ListAudits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) < 2 || audits[len(audits)-1].PolicyID != policy.ID || audits[len(audits)-1].EventType != "policy_deleted_passthrough" || !strings.Contains(string(audits[len(audits)-1].Details), PassthroughConfirmation) {
		t.Fatalf("delete audit was not retained with the permission-change confirmation: %#v", audits)
	}
}

func TestPassthroughUnavailableAndNoFallbackAreDistinct(t *testing.T) {
	service := newTestService(t)
	unconfigured := testIdentity(t, "unconfigured")
	decision, err := service.Decide(unconfigured)
	if err != nil || decision.Mode != ModePassthrough || decision.Snapshot != nil {
		t.Fatalf("passthrough = %#v, %v", decision, err)
	}
	service.MarkUnavailable()
	if _, err := service.Decide(unconfigured); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable error = %v", err)
	}
}

func TestRequestDecisionNormalizesThinkingSuffixWithoutBroadeningModelAccess(t *testing.T) {
	decision := RequestPolicyDecision{Mode: ModeProfile, Snapshot: &RequestPolicySnapshot{
		ModelMappings: map[string]string{"smart": "gpt-5"},
		AllowedModels: map[string]struct{}{"gpt-5": {}, "custom(8192)": {}},
	}}
	tests := []struct {
		name      string
		requested string
		want      string
		wantErr   bool
	}{
		{name: "allowed base with suffix", requested: "gpt-5(high)", want: "gpt-5(high)"},
		{name: "mapped base with suffix", requested: "smart(8192)", want: "gpt-5(8192)"},
		{name: "exact custom parenthesized model", requested: "custom(8192)", want: "custom(8192)"},
		{name: "forbidden base cannot use suffix", requested: "claude-opus(high)", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decision.ApplyModel(test.requested)
			if test.wantErr {
				if err == nil {
					t.Fatalf("ApplyModel(%q) unexpectedly succeeded with %q", test.requested, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("ApplyModel(%q) = %q, %v; want %q", test.requested, got, err, test.want)
			}
		})
	}
}

func TestEmptyProviderAndModelSelectionsAllowAllCatalogAndFutureValues(t *testing.T) {
	service := newTestService(t)
	service.SetCatalogProvider(func() (ProfileCatalog, error) {
		return NewProfileCatalog([]string{"codex"}, []string{"gpt-5"}), nil
	})
	identity := testIdentity(t, "allow-all-key")
	policy, err := service.Create(context.Background(), identity, "Allow all", ProfileInput{
		Name: "Open", Mappings: []ModelMapping{{Source: "smart", Target: "gpt-5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Profiles) != 1 || len(policy.Profiles[0].Providers) != 0 || len(policy.Profiles[0].Models) != 0 {
		t.Fatalf("persisted wildcard profile=%#v", policy.Profiles)
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Snapshot == nil {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	for requested, want := range map[string]string{
		"future-model": "future-model",
		"smart":        "gpt-5",
		"smart(high)":  "gpt-5(high)",
	} {
		if got, applyErr := decision.ApplyModel(requested); applyErr != nil || got != want {
			t.Fatalf("ApplyModel(%q)=%q, %v; want %q", requested, got, applyErr, want)
		}
	}
	if err = decision.AllowsProvider("future-provider"); err != nil {
		t.Fatalf("future provider rejected: %v", err)
	}
	providers, err := decision.FilterProviders([]string{"Future-Provider", "codex", "future-provider"})
	if err != nil || len(providers) != 2 || providers[0] != "future-provider" || providers[1] != "codex" {
		t.Fatalf("filtered providers=%#v err=%v", providers, err)
	}
	visible, err := decision.FilterVisibleModels([]ModelCandidate{
		{ID: "future-model", Providers: []string{"future-provider"}},
		{ID: "gpt-5", Providers: []string{"codex"}},
	})
	if err != nil || len(visible) != 3 || visible[0].ID != "future-model" || visible[1].ID != "gpt-5" || visible[2] != (VisibleModel{ID: "smart", EffectiveID: "gpt-5"}) {
		t.Fatalf("visible models=%#v err=%v", visible, err)
	}
	invalid := ProfileInput{Name: "Open", Mappings: []ModelMapping{{Source: "smart", Target: "unknown"}}}
	if _, err = service.Create(context.Background(), testIdentity(t, "invalid-open-key"), "", invalid); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("unknown wildcard mapping target error=%v", err)
	}
}

func TestRequestDecisionExactAutoAndPrefixedMappingsWinBeforeNormalization(t *testing.T) {
	decision := RequestPolicyDecision{Mode: ModeProfile, Snapshot: &RequestPolicySnapshot{
		ModelMappings: map[string]string{"auto": "gpt-5", "team/gpt": "gpt-5"},
		AllowedModels: map[string]struct{}{"gpt-5": {}},
	}}
	for source, want := range map[string]string{"auto": "gpt-5", "team/gpt": "gpt-5"} {
		if !decision.HasExactModelMapping(source) {
			t.Fatalf("HasExactModelMapping(%q) = false", source)
		}
		if got, err := decision.ApplyModel(source); err != nil || got != want {
			t.Fatalf("ApplyModel(%q) = %q, %v; want %q", source, got, err, want)
		}
	}
	if decision.HasExactModelMapping("auto(high)") {
		t.Fatal("suffix must not be confused with an exact mapping")
	}
	if got, err := decision.ApplyModel("auto(high)"); err != nil || got != "gpt-5(high)" {
		t.Fatalf("ApplyModel(auto(high)) = %q, %v; want gpt-5(high)", got, err)
	}
}

func TestRequestDecisionValidatesRouterTargetWithoutApplyingAliasAgain(t *testing.T) {
	decision := RequestPolicyDecision{Mode: ModeProfile, Snapshot: &RequestPolicySnapshot{
		ModelMappings: map[string]string{"smart": "gpt-5", "gpt-5": "claude-opus"},
		AllowedModels: map[string]struct{}{"gpt-5": {}, "claude-opus": {}},
	}}
	if got, err := decision.ValidateEffectiveModel("gpt-5(high)"); err != nil || got != "gpt-5(high)" {
		t.Fatalf("ValidateEffectiveModel() = %q, %v", got, err)
	}
	if _, err := decision.ValidateEffectiveModel("gemini-pro(high)"); err == nil {
		t.Fatal("forbidden router target unexpectedly passed validation")
	}
}

func TestListOrphanedPageUsesStableDatabaseCursor(t *testing.T) {
	service := newTestService(t)
	configured := testIdentity(t, "configured-key")
	if _, err := service.Create(context.Background(), configured, "Configured", ProfileInput{Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"}}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"orphan-a", "orphan-b", "orphan-c"} {
		if _, err := service.Create(context.Background(), testIdentity(t, key), key, ProfileInput{Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"}}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := service.ListOrphanedPage(context.Background(), []string{configured.Hash()}, 0, "", 2)
	if err != nil || len(first) != 2 {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := service.ListOrphanedPage(context.Background(), []string{configured.Hash()}, first[1].CreatedAtMS, first[1].ID, 2)
	if err != nil || len(second) != 1 || second[0].ID == first[0].ID || second[0].ID == first[1].ID {
		t.Fatalf("second page = %#v, %v", second, err)
	}
}

func TestProfileValidationAndOptimisticVersion(t *testing.T) {
	service := newTestService(t)
	identity := testIdentity(t, "key-b")
	policy, err := service.Create(context.Background(), identity, "", ProfileInput{
		Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProfile(context.Background(), policy.ID, policy.Version-1, ProfileInput{Name: "bad", Providers: []string{"codex"}, Models: []string{"gpt-5"}}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("version error = %v", err)
	}
	if _, err := service.CreateProfile(context.Background(), policy.ID, policy.Version, ProfileInput{Name: "bad", Providers: []string{"codex"}, Models: []string{"gpt-5"}, Mappings: []ModelMapping{{Source: "a", Target: "b"}}}); err == nil {
		t.Fatal("accepted mapping to a non-allowed target")
	}
	if err := service.DeletePolicy(context.Background(), policy.ID, policy.Version, "ordinary-delete"); !errors.Is(err, ErrPassthroughConfirmation) {
		t.Fatalf("delete confirmation error = %v", err)
	}
	if err := service.DeletePolicy(context.Background(), policy.ID, policy.Version, PassthroughConfirmation); err != nil {
		t.Fatal(err)
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Mode != ModePassthrough {
		t.Fatalf("decision after confirmed delete = %#v, %v", decision, err)
	}
}

func TestConcurrentWritesWithSameVersionHaveOneWinner(t *testing.T) {
	service := newTestService(t)
	policy, err := service.Create(context.Background(), testIdentity(t, "concurrent-version-key"), "before", ProfileInput{
		Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	const writers = 12
	start := make(chan struct{})
	errorsByWriter := make(chan error, writers)
	var ready sync.WaitGroup
	ready.Add(writers)
	for writer := 0; writer < writers; writer++ {
		writer := writer
		go func() {
			ready.Done()
			<-start
			_, updateErr := service.UpdateDisplayName(context.Background(), policy.ID, "writer-"+string(rune('a'+writer)), policy.Version)
			errorsByWriter <- updateErr
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	conflicts := 0
	for writer := 0; writer < writers; writer++ {
		updateErr := <-errorsByWriter
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent write returned unexpected error: %v", updateErr)
		}
	}
	if successes != 1 || conflicts != writers-1 {
		t.Fatalf("successes=%d conflicts=%d; want 1 and %d", successes, conflicts, writers-1)
	}
	stored, err := service.Get(context.Background(), policy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != policy.Version+1 || !strings.HasPrefix(stored.DisplayName, "writer-") {
		t.Fatalf("stored policy after concurrent writes = %#v", stored)
	}
}

func TestWorkspaceUpdateIsAtomicAndIncrementsVersionOnce(t *testing.T) {
	service := newTestService(t)
	policy, err := service.Create(context.Background(), testIdentity(t, "workspace-key"), "Before", ProfileInput{
		Name: "Production", Providers: []string{"codex"}, Models: []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := ProfileInput{Name: "Production v2", Providers: []string{"claude"}, Models: []string{"claude-sonnet-4-6"}}
	updated, err := service.UpdateWorkspace(context.Background(), policy.ID, policy.Version, WorkspaceUpdate{
		DisplayName: "After", ProfileID: policy.ActiveProfileID, Profile: &profile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != policy.Version+1 || updated.DisplayName != "After" || len(updated.Profiles) != 1 || updated.Profiles[0].Name != "Production v2" {
		t.Fatalf("atomic update = %#v", updated)
	}

	failedProfile := ProfileInput{Name: "Should not persist", Providers: []string{"codex"}, Models: []string{"gpt-5"}}
	if _, err := service.UpdateWorkspace(context.Background(), policy.ID, updated.Version, WorkspaceUpdate{
		DisplayName: "Must roll back", ProfileID: "missing-profile", Profile: &failedProfile,
	}); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing profile error = %v", err)
	}
	stored, err := service.Get(context.Background(), policy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != updated.Version || stored.DisplayName != updated.DisplayName || stored.Profiles[0].Name != updated.Profiles[0].Name {
		t.Fatalf("failed workspace write partially committed: %#v", stored)
	}
}

func TestProfileCatalogRejectsUnknownProviderAndModel(t *testing.T) {
	service := newTestService(t)
	service.SetCatalogProvider(func() (ProfileCatalog, error) {
		return NewProfileCatalog(
			[]string{"codex", "home"},
			[]string{"gpt-5", "home-model"},
			map[string][]string{"gpt-5": {"codex"}, "home-model": {"home"}},
		), nil
	})
	identity := testIdentity(t, "catalog-key")
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "bad-provider", Providers: []string{"claude"}, Models: []string{"gpt-5"}}); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("unknown provider error = %v", err)
	}
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "bad-model", Providers: []string{"codex"}, Models: []string{"gpt-unknown"}}); err == nil || !strings.Contains(err.Error(), "unknown model") {
		t.Fatalf("unknown model error = %v", err)
	}
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "wrong-provider", Providers: []string{"home"}, Models: []string{"gpt-5"}}); err == nil || !strings.Contains(err.Error(), "not available from an allowed provider") {
		t.Fatalf("provider/model mismatch error = %v", err)
	}
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "wrong-mapping-provider", Providers: []string{"home"}, Mappings: []ModelMapping{{Source: "alias", Target: "gpt-5"}}}); err == nil || !strings.Contains(err.Error(), "not available from an allowed provider") {
		t.Fatalf("provider/mapping mismatch error = %v", err)
	}
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "valid", Providers: []string{"Codex"}, Models: []string{"gpt-5"}}); err != nil {
		t.Fatal(err)
	}
}

func TestUsageAttributionDoesNotInventPassthroughProfile(t *testing.T) {
	passthrough := PassthroughDecision().UsageAttribution()
	if passthrough.PolicyMode != ModePassthrough || passthrough.APIKeyPolicyID != "" || passthrough.ProfileID != "" {
		t.Fatalf("passthrough attribution = %#v", passthrough)
	}
	decision := RequestPolicyDecision{Mode: ModeProfile, Snapshot: &RequestPolicySnapshot{
		PolicyID: "policy-a", ProfileID: "profile-a", ProfileName: "Production",
		RequestedModel: "smart", EffectiveModel: "gpt-5",
	}}
	attribution := decision.UsageAttribution()
	if attribution.PolicyMode != ModeProfile || attribution.APIKeyPolicyID != "policy-a" || attribution.ProfileID != "profile-a" || attribution.ProfileName != "Production" || attribution.RequestedModel != "smart" || attribution.EffectiveModel != "gpt-5" {
		t.Fatalf("profile attribution = %#v", attribution)
	}
}

func TestPolicyBackupRoundTripAndValidation(t *testing.T) {
	service := newTestService(t)
	service.SetCatalogProvider(func() (ProfileCatalog, error) {
		return NewProfileCatalog([]string{"codex"}, []string{"gpt-5"}), nil
	})
	identity := testIdentity(t, "backup-key")
	created, err := service.Create(context.Background(), identity, "Backup key", ProfileInput{Name: "Production", Providers: []string{"codex"}, Models: []string{"gpt-5"}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := service.ExportBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "backup-key") || !strings.Contains(string(payload), identity.Hash()) {
		t.Fatalf("backup secret/hash boundary = %s", payload)
	}
	if !strings.Contains(string(payload), `"schema_version":3`) || !strings.Contains(string(payload), `"takeover_enabled":true`) || !strings.Contains(string(payload), `"eventType":"policy_created"`) {
		t.Fatalf("backup schema/audit record = %s", payload)
	}
	if err := service.DeletePolicy(context.Background(), created.ID, created.Version, PassthroughConfirmation); err != nil {
		t.Fatal(err)
	}
	service.SetCatalogProvider(func() (ProfileCatalog, error) { return ProfileCatalog{}, nil })
	if _, err := service.PreviewBackup(context.Background(), payload, []string{identity.Hash()}); err != nil {
		t.Fatalf("preview backup without a live catalog: %v", err)
	}
	if err := service.ImportBackup(context.Background(), payload); err != nil {
		t.Fatalf("restore backup without a live catalog: %v", err)
	}
	decision, err := service.Decide(identity)
	if err != nil || decision.Mode != ModeProfile || decision.Snapshot == nil || decision.Snapshot.ProfileName != "Production" {
		t.Fatalf("restored decision = %#v, %v", decision, err)
	}
	audits, err := service.store.ListAudits(context.Background())
	if err != nil || len(audits) != 1 || audits[0].PolicyID != created.ID || audits[0].EventType != "policy_created" {
		t.Fatalf("restored audits = %#v, %v", audits, err)
	}
	var invalid backupDocument
	if err := json.Unmarshal(payload, &invalid); err != nil {
		t.Fatal(err)
	}
	invalid.Policies[0].Profiles[0].Name = ""
	invalidPayload, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ImportBackup(context.Background(), invalidPayload); err == nil || !strings.Contains(err.Error(), "profile name is required") {
		t.Fatalf("invalid structural import error = %v", err)
	}
	decision, err = service.Decide(identity)
	if err != nil || decision.Mode != ModeProfile || decision.Snapshot.ProfileName != "Production" {
		t.Fatalf("failed import changed live snapshot = %#v, %v", decision, err)
	}
}

func TestPolicyBackupPreviewUsesCommittedConfiguredKeysAndLegacyArrayStillImports(t *testing.T) {
	service := newTestService(t)
	service.SetCatalogProvider(func() (ProfileCatalog, error) {
		return NewProfileCatalog([]string{"codex"}, []string{"gpt-5"}), nil
	})
	first := testIdentity(t, "configured-key")
	second := testIdentity(t, "missing-key")
	for _, identity := range []AuthenticatedAPIKeyIdentity{first, second} {
		if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: identity.Hash()[:8], Providers: []string{"codex"}, Models: []string{"gpt-5"}}); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := service.ExportBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewBackup(context.Background(), payload, []string{first.Hash()})
	if err != nil || preview.TargetPolicies != 2 || preview.TargetProfiles != 2 || preview.ReplacePolicies != 2 || preview.AssociatedPolicies != 1 || preview.OrphanedPolicies != 1 || !preview.CurrentTakeoverEnabled || !preview.TargetTakeoverEnabled {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	var document backupDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(document.Policies)
	if err != nil {
		t.Fatal(err)
	}
	legacyPreview, err := service.PreviewBackup(context.Background(), legacy, []string{first.Hash()})
	if err != nil || !legacyPreview.CurrentTakeoverEnabled || legacyPreview.TargetTakeoverEnabled {
		t.Fatalf("legacy preview = %+v, %v", legacyPreview, err)
	}
	if err := service.ImportBackup(context.Background(), legacy); err != nil {
		t.Fatalf("legacy policy array import = %v", err)
	}
}

func TestPolicyBackupPreviewAndImportShareCanonicalStagedProfiles(t *testing.T) {
	service := newTestService(t)
	service.SetCatalogProvider(func() (ProfileCatalog, error) {
		return NewProfileCatalog([]string{"codex"}, []string{"gpt-5"}), nil
	})
	identity := testIdentity(t, "canonical-backup-key")
	if _, err := service.Create(context.Background(), identity, "", ProfileInput{Name: " Production ", Providers: []string{"codex"}, Models: []string{"gpt-5"}}); err != nil {
		t.Fatal(err)
	}
	payload, err := service.ExportBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(payload), `"providers":["codex"]`, `"providers":[" CODEX ","codex"]`, 1)
	if mutated == string(payload) {
		t.Fatalf("provider fixture was not mutated: %s", payload)
	}
	if _, err = service.PreviewBackup(context.Background(), []byte(mutated), []string{identity.Hash()}); err != nil {
		t.Fatalf("canonical preview failed: %v", err)
	}
	if err = service.ImportBackup(context.Background(), []byte(mutated)); err != nil {
		t.Fatalf("canonical import failed after preview: %v", err)
	}
	policies, err := service.List(context.Background())
	if err != nil || len(policies) != 1 || len(policies[0].Profiles) != 1 {
		t.Fatalf("imported policies=%#v, err=%v", policies, err)
	}
	profile := policies[0].Profiles[0]
	if profile.Name != "Production" || len(profile.Providers) != 1 || profile.Providers[0] != "codex" {
		t.Fatalf("persisted non-canonical profile: %#v", profile)
	}
}

func TestForeignKeysRejectOrphansAndCascadeProfiles(t *testing.T) {
	service := newTestService(t)
	if _, err := service.store.db.Exec(`insert into api_key_profiles(id, policy_id, name, created_at_ms, updated_at_ms) values('orphan', 'missing', 'bad', 1, 1)`); err == nil {
		t.Fatal("foreign keys accepted an orphan profile")
	}
	identity := testIdentity(t, "key-c")
	policy, err := service.Create(context.Background(), identity, "", ProfileInput{Name: "default", Providers: []string{"codex"}, Models: []string{"gpt-5"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeletePolicy(context.Background(), policy.ID, policy.Version, PassthroughConfirmation); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := service.store.db.QueryRow(`select count(*) from api_key_profiles where policy_id = ?`, policy.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("profile count = %d, want cascade delete", count)
	}
}

func TestVisibleModelsRequireAllowedProviderAndExposeMappingAliases(t *testing.T) {
	decision := RequestPolicyDecision{Mode: ModeProfile, Snapshot: &RequestPolicySnapshot{
		ModelMappings:    map[string]string{"smart": "gpt-5"},
		AllowedModels:    map[string]struct{}{"gpt-5": {}, "claude-opus": {}},
		AllowedProviders: map[string]struct{}{"codex": {}},
	}}
	visible, err := decision.FilterVisibleModels([]ModelCandidate{
		{ID: "gpt-5", Providers: []string{"codex", "openai-compatibility"}},
		{ID: "claude-opus", Providers: []string{"claude"}},
		{ID: "unlisted", Providers: []string{"codex"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 || visible[0] != (VisibleModel{ID: "gpt-5", EffectiveID: "gpt-5"}) || visible[1] != (VisibleModel{ID: "smart", EffectiveID: "gpt-5"}) {
		t.Fatalf("visible models = %#v", visible)
	}
}
