package verbosity

import (
	"testing"
)

func TestVerbosityLevel_Valid(t *testing.T) {
	levels := []VerbosityLevel{
		VerbosityQuiet, VerbosityMinimal, VerbosityNormal,
		VerbosityDetailed, VerbosityDebug,
	}
	for _, lvl := range levels {
		if !IsValidVerbosityLevel(string(lvl)) {
			t.Errorf("expected %q to be valid", lvl)
		}
	}
}

func TestVerbosityLevel_Invalid(t *testing.T) {
	if IsValidVerbosityLevel("verbose") {
		t.Error("expected 'verbose' to be invalid")
	}
	if IsValidVerbosityLevel("") {
		t.Error("expected empty string to be invalid")
	}
}

func TestShouldShow_Quiet(t *testing.T) {
	// TierQuiet content visible at all levels
	for _, lvl := range []VerbosityLevel{VerbosityQuiet, VerbosityMinimal, VerbosityNormal, VerbosityDetailed, VerbosityDebug} {
		if !ShouldShow(TierQuiet, lvl) {
			t.Errorf("TierQuiet should show at %s", lvl)
		}
	}
}

func TestShouldShow_Normal(t *testing.T) {
	tests := []struct {
		verbosity VerbosityLevel
		want      bool
	}{
		{VerbosityQuiet, false},
		{VerbosityMinimal, true},
		{VerbosityNormal, true},
		{VerbosityDetailed, true},
		{VerbosityDebug, true},
	}
	for _, tt := range tests {
		got := ShouldShow(TierNormal, tt.verbosity)
		if got != tt.want {
			t.Errorf("ShouldShow(TierNormal, %s) = %v, want %v", tt.verbosity, got, tt.want)
		}
	}
}

func TestShouldShow_Detailed(t *testing.T) {
	tests := []struct {
		verbosity VerbosityLevel
		want      bool
	}{
		{VerbosityQuiet, false},
		{VerbosityMinimal, false},
		{VerbosityNormal, false},
		{VerbosityDetailed, true},
		{VerbosityDebug, true},
	}
	for _, tt := range tests {
		got := ShouldShow(TierDetailed, tt.verbosity)
		if got != tt.want {
			t.Errorf("ShouldShow(TierDetailed, %s) = %v, want %v", tt.verbosity, got, tt.want)
		}
	}
}

func TestShouldShow_Debug(t *testing.T) {
	tests := []struct {
		verbosity VerbosityLevel
		want      bool
	}{
		{VerbosityQuiet, false},
		{VerbosityNormal, false},
		{VerbosityDetailed, false},
		{VerbosityDebug, true},
	}
	for _, tt := range tests {
		got := ShouldShow(TierDebug, tt.verbosity)
		if got != tt.want {
			t.Errorf("ShouldShow(TierDebug, %s) = %v, want %v", tt.verbosity, got, tt.want)
		}
	}
}

func TestShouldShow_Internal(t *testing.T) {
	// TierInternal is never shown
	for _, lvl := range []VerbosityLevel{VerbosityQuiet, VerbosityMinimal, VerbosityNormal, VerbosityDetailed, VerbosityDebug} {
		if ShouldShow(TierInternal, lvl) {
			t.Errorf("TierInternal should never show, but showed at %s", lvl)
		}
	}
}

func TestShouldShow_InvalidVerbosity(t *testing.T) {
	// Invalid verbosity defaults to normal (tier 1)
	got := ShouldShow(TierQuiet, "invalid")
	if !got {
		t.Error("TierQuiet should show at default verbosity")
	}
	got = ShouldShow(TierDetailed, "invalid")
	if got {
		t.Error("TierDetailed should not show at default verbosity")
	}
}

func TestVerbosityLevelValues_MinimalEqualsNormal(t *testing.T) {
	// minimal and normal map to the same integer value
	if verbosityLevelValues[VerbosityMinimal] != verbosityLevelValues[VerbosityNormal] {
		t.Error("minimal and normal should map to the same tier value")
	}
}
