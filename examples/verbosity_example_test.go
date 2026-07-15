package soothe_test

import (
	"fmt"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_shouldShow demonstrates verbosity-based content visibility filtering.
// Content at a given tier is only visible if the user's verbosity level is high enough.
func Example_shouldShow() {
	// At quiet verbosity, only TierQuiet content is visible.
	fmt.Println(soothe.ShouldShow(soothe.TierQuiet, soothe.VerbosityQuiet))    // true
	fmt.Println(soothe.ShouldShow(soothe.TierNormal, soothe.VerbosityQuiet))   // false
	fmt.Println(soothe.ShouldShow(soothe.TierDetailed, soothe.VerbosityQuiet)) // false

	// At normal verbosity, quiet + normal tiers are visible.
	fmt.Println(soothe.ShouldShow(soothe.TierQuiet, soothe.VerbosityNormal))    // true
	fmt.Println(soothe.ShouldShow(soothe.TierNormal, soothe.VerbosityNormal))   // true
	fmt.Println(soothe.ShouldShow(soothe.TierDetailed, soothe.VerbosityNormal)) // false

	// At debug verbosity, everything except TierInternal is visible.
	fmt.Println(soothe.ShouldShow(soothe.TierDebug, soothe.VerbosityDebug))    // true
	fmt.Println(soothe.ShouldShow(soothe.TierInternal, soothe.VerbosityDebug)) // false
	// Output:
	// true
	// false
	// false
	// true
	// true
	// false
	// true
	// false
}

// Example_isValidVerbosityLevel checks whether a string is a recognized
// verbosity level ("quiet", "normal", or "debug").
func Example_isValidVerbosityLevel() {
	fmt.Println(soothe.IsValidVerbosityLevel("quiet"))  // true
	fmt.Println(soothe.IsValidVerbosityLevel("normal")) // true
	fmt.Println(soothe.IsValidVerbosityLevel("debug"))  // true
	fmt.Println(soothe.IsValidVerbosityLevel("trace"))  // false
	fmt.Println(soothe.IsValidVerbosityLevel(""))       // false
	// Output:
	// true
	// true
	// true
	// false
	// false
}

// Example_verbosityLevels shows the typed verbosity level and tier constants.
func Example_verbosityLevels() {
	// VerbosityLevel is a string type.
	level := soothe.VerbosityDebug
	fmt.Println(level) // debug

	// VerbosityTier is an int type.
	fmt.Println(int(soothe.TierQuiet))    // 0
	fmt.Println(int(soothe.TierNormal))   // 1
	fmt.Println(int(soothe.TierDetailed)) // 2
	fmt.Println(int(soothe.TierDebug))    // 3
	fmt.Println(int(soothe.TierInternal)) // 99
	// Output:
	// debug
	// 0
	// 1
	// 2
	// 3
	// 99
}
