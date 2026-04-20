package verbosity

// VerbosityLevel represents the user-configurable verbosity setting.
type VerbosityLevel string

const (
	VerbosityQuiet    VerbosityLevel = "quiet"
	VerbosityMinimal  VerbosityLevel = "minimal"
	VerbosityNormal   VerbosityLevel = "normal"
	VerbosityDetailed VerbosityLevel = "detailed"
	VerbosityDebug    VerbosityLevel = "debug"
)

// VerbosityTier represents the minimum verbosity level at which content is visible.
// Lower tier values mean the content is visible at lower verbosity settings.
type VerbosityTier int

const (
	TierQuiet    VerbosityTier = 0  // Always visible (errors, assistant text, final reports)
	TierNormal   VerbosityTier = 1  // Standard progress (plan updates, milestones, agentic loop)
	TierDetailed VerbosityTier = 2  // Detailed internals (protocol events, tool calls, subagent activity)
	TierDebug    VerbosityTier = 3  // Everything including internals (thinking, heartbeats)
	TierInternal VerbosityTier = 99 // Never shown at any level (implementation details)
)

// verbosityLevelValues maps VerbosityLevel strings to their integer values.
var verbosityLevelValues = map[VerbosityLevel]int{
	VerbosityQuiet:    0,
	VerbosityMinimal:  1,
	VerbosityNormal:   1,
	VerbosityDetailed: 2,
	VerbosityDebug:    3,
}

// ShouldShow returns true if content at the given tier is visible at the given verbosity.
func ShouldShow(tier VerbosityTier, verbosity VerbosityLevel) bool {
	if tier == TierInternal {
		return false
	}
	level, ok := verbosityLevelValues[verbosity]
	if !ok {
		level = 1 // default to normal
	}
	return int(tier) <= level
}

// IsValidVerbosityLevel checks whether a string is a valid verbosity level.
func IsValidVerbosityLevel(s string) bool {
	_, ok := verbosityLevelValues[VerbosityLevel(s)]
	return ok
}
