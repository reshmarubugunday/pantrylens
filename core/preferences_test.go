package core

import "testing"

func TestPreferenceStoreRoundTrip(t *testing.T) {
	s := NewPreferenceStore()
	if _, ok := s.Get("user-1"); ok {
		t.Fatal("expected no preferences for a user that hasn't saved any")
	}

	s.Save("user-1", Preferences{LastServings: 4, LastCuisine: "Thai"})
	prefs, ok := s.Get("user-1")
	if !ok {
		t.Fatal("expected preferences after saving")
	}
	if prefs.LastServings != 4 || prefs.LastCuisine != "Thai" {
		t.Errorf("got %+v", prefs)
	}
}

func TestPreferenceStoreScopedPerUser(t *testing.T) {
	s := NewPreferenceStore()
	s.Save("user-1", Preferences{LastServings: 2, LastCuisine: "Italian"})
	if _, ok := s.Get("user-2"); ok {
		t.Error("user-2 should not see user-1's saved preferences")
	}
}
