package testdesign

import (
	"fmt"
	"strings"
)

// Validator validates test suites and test cases
type Validator struct {
	config ValidationConfig
}

// ValidationConfig configures validation rules
type ValidationConfig struct {
	RequireSelectors      bool
	RequireAssertions     bool
	RequireBDD            bool
	MinStepsPerTest       int
	MaxStepsPerTest       int
	RequireTestData       bool
	ValidateSelectors     bool
	RequireComplianceRefs bool
}

// DefaultValidationConfig returns sensible defaults
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		RequireSelectors:      true,
		RequireAssertions:     true,
		RequireBDD:            true,
		MinStepsPerTest:       2,
		MaxStepsPerTest:       50,
		RequireTestData:       false,
		ValidateSelectors:     true,
		RequireComplianceRefs: false,
	}
}

// NewValidator creates a new validator with default config
func NewValidator() *Validator {
	return &Validator{
		config: DefaultValidationConfig(),
	}
}

// NewValidatorWithConfig creates a new validator with custom config
func NewValidatorWithConfig(config ValidationConfig) *Validator {
	return &Validator{
		config: config,
	}
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error
type ValidationError struct {
	Path    string
	Code    string
	Message string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Path    string
	Code    string
	Message string
}

// ValidateSuite validates an entire test suite
func (v *Validator) ValidateSuite(suite *TestSuite) []string {
	var warnings []string

	if suite.ID == "" {
		warnings = append(warnings, "Suite: Missing ID")
	}

	if suite.Name == "" {
		warnings = append(warnings, "Suite: Missing name")
	}

	if len(suite.Features) == 0 {
		warnings = append(warnings, "Suite: No features generated")
	}

	// Validate each feature
	for i, feature := range suite.Features {
		featureWarnings := v.ValidateFeature(&feature, fmt.Sprintf("Feature[%d]", i))
		warnings = append(warnings, featureWarnings...)
	}

	// Check for duplicate test IDs
	testIDs := make(map[string]int)
	for _, feature := range suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				testIDs[tc.ID]++
				if testIDs[tc.ID] > 1 {
					warnings = append(warnings, fmt.Sprintf("Duplicate test ID: %s", tc.ID))
				}
			}
		}
	}

	// Check coverage
	coverageWarnings := v.checkCoverage(suite)
	warnings = append(warnings, coverageWarnings...)

	return warnings
}

// ValidateFeature validates a feature
func (v *Validator) ValidateFeature(feature *Feature, path string) []string {
	var warnings []string

	if feature.ID == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing ID", path))
	}

	if feature.Name == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing name", path))
	}

	if len(feature.Scenarios) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s: No scenarios", path))
	}

	// Validate scenarios
	for i, scenario := range feature.Scenarios {
		scenarioWarnings := v.ValidateScenario(&scenario, fmt.Sprintf("%s.Scenario[%d]", path, i))
		warnings = append(warnings, scenarioWarnings...)
	}

	return warnings
}

// ValidateScenario validates a scenario
func (v *Validator) ValidateScenario(scenario *Scenario, path string) []string {
	var warnings []string

	if scenario.ID == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing ID", path))
	}

	if scenario.Name == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing name", path))
	}

	if len(scenario.TestCases) == 0 {
		warnings = append(warnings, fmt.Sprintf("%s: No test cases", path))
	}

	// Validate test cases
	for i, tc := range scenario.TestCases {
		tcWarnings := v.ValidateTestCase(&tc, fmt.Sprintf("%s.TestCase[%d]", path, i))
		warnings = append(warnings, tcWarnings...)
	}

	return warnings
}

// ValidateTestCase validates a test case
func (v *Validator) ValidateTestCase(tc *TestCase, path string) []string {
	var warnings []string

	// Required fields
	if tc.ID == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing ID", path))
	}

	if tc.Name == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing name", path))
	}

	// BDD validation
	if v.config.RequireBDD {
		if tc.Given == "" {
			warnings = append(warnings, fmt.Sprintf("%s: Missing Given clause", path))
		}
		if tc.When == "" {
			warnings = append(warnings, fmt.Sprintf("%s: Missing When clause", path))
		}
		if tc.Then == "" {
			warnings = append(warnings, fmt.Sprintf("%s: Missing Then clause", path))
		}
	}

	// Steps validation
	if len(tc.Steps) < v.config.MinStepsPerTest {
		warnings = append(warnings, fmt.Sprintf("%s: Too few steps (%d < %d)", path, len(tc.Steps), v.config.MinStepsPerTest))
	}
	if len(tc.Steps) > v.config.MaxStepsPerTest {
		warnings = append(warnings, fmt.Sprintf("%s: Too many steps (%d > %d)", path, len(tc.Steps), v.config.MaxStepsPerTest))
	}

	// Validate steps
	for i, step := range tc.Steps {
		stepWarnings := v.ValidateTestStep(&step, fmt.Sprintf("%s.Step[%d]", path, i))
		warnings = append(warnings, stepWarnings...)
	}

	// Check for assertions
	if v.config.RequireAssertions {
		hasAssertion := false
		for _, step := range tc.Steps {
			if step.Action == ActionAssert || len(step.Assertions) > 0 {
				hasAssertion = true
				break
			}
		}
		if !hasAssertion {
			warnings = append(warnings, fmt.Sprintf("%s: No assertions found", path))
		}
	}

	return warnings
}

// ValidateTestStep validates a test step
func (v *Validator) ValidateTestStep(step *TestStep, path string) []string {
	var warnings []string

	if step.Action == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing action", path))
	}

	if step.Description == "" {
		warnings = append(warnings, fmt.Sprintf("%s: Missing description", path))
	}

	// Actions that require a target
	actionsRequiringTarget := map[Action]bool{
		ActionClick:       true,
		ActionDoubleClick: true,
		ActionRightClick:  true,
		ActionFill:        true,
		ActionSelect:      true,
		ActionCheck:       true,
		ActionUncheck:     true,
		ActionHover:       true,
	}

	if actionsRequiringTarget[step.Action] {
		if step.Target == "" && step.Selectors == nil {
			warnings = append(warnings, fmt.Sprintf("%s: Action '%s' requires a target", path, step.Action))
		}
	}

	// Actions that require a value
	actionsRequiringValue := map[Action]bool{
		ActionFill:     true,
		ActionSelect:   true,
		ActionNavigate: true,
	}

	if actionsRequiringValue[step.Action] {
		if step.Value == "" && step.ValueFrom == "" && step.Target == "" {
			// For navigate, target URL is in Target field
			if step.Action != ActionNavigate {
				warnings = append(warnings, fmt.Sprintf("%s: Action '%s' requires a value", path, step.Action))
			}
		}
	}

	// Validate selectors
	if v.config.ValidateSelectors && step.Selectors != nil {
		selectorWarnings := v.ValidateSelectors(step.Selectors, path)
		warnings = append(warnings, selectorWarnings...)
	}

	return warnings
}

// ValidateSelectors validates selector candidates
func (v *Validator) ValidateSelectors(selectors *SelectorCandidates, path string) []string {
	var warnings []string

	if selectors.Primary == "" {
		warnings = append(warnings, fmt.Sprintf("%s.Selectors: Missing primary selector", path))
	}

	// Check selector quality
	if selectors.Primary != "" {
		quality := v.assessSelectorQuality(selectors.Primary)
		if quality < 0.5 {
			warnings = append(warnings, fmt.Sprintf("%s.Selectors: Low quality primary selector: %s", path, selectors.Primary))
		}
	}

	// Check for fallbacks
	if len(selectors.Fallbacks) == 0 && selectors.Confidence < 0.9 {
		warnings = append(warnings, fmt.Sprintf("%s.Selectors: No fallback selectors for low-confidence primary", path))
	}

	return warnings
}

// assessSelectorQuality scores a selector from 0-1
func (v *Validator) assessSelectorQuality(selector string) float64 {
	score := 0.5 // Base score

	// Prefer data-testid
	if strings.Contains(selector, "data-testid") {
		score = 1.0
	} else if strings.Contains(selector, "data-test") {
		score = 0.95
	} else if strings.HasPrefix(selector, "#") {
		// ID selector - good
		score = 0.8
	} else if strings.Contains(selector, "[aria-") {
		// ARIA selector - good for accessibility
		score = 0.85
	} else if strings.Contains(selector, "[name=") {
		// Name attribute - decent
		score = 0.7
	} else if strings.HasPrefix(selector, ".") {
		// Class selector - less stable
		score = 0.5
	} else if strings.Contains(selector, " > ") || strings.Contains(selector, " ") {
		// Complex selector - less maintainable
		score = 0.3
	}

	// Penalize very long selectors
	if len(selector) > 100 {
		score *= 0.8
	}

	// Penalize selectors with specific index
	if strings.Contains(selector, ":nth-") {
		score *= 0.7
	}

	return score
}

// checkCoverage checks test coverage across different dimensions
func (v *Validator) checkCoverage(suite *TestSuite) []string {
	var warnings []string

	// Count test types
	typeCount := make(map[TestType]int)
	priorityCount := make(map[Priority]int)
	categoryCount := make(map[TestCategory]int)

	for _, feature := range suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				typeCount[tc.Type]++
				priorityCount[tc.Priority]++
				categoryCount[tc.Category]++
			}
		}
	}

	// Check for missing test types
	recommendedTypes := []TestType{
		TestTypeSmoke,
		TestTypeRegression,
		TestTypeNegative,
	}

	for _, t := range recommendedTypes {
		if typeCount[t] == 0 {
			warnings = append(warnings, fmt.Sprintf("Coverage: No %s tests", t))
		}
	}

	// Check priority distribution
	totalTests := 0
	for _, count := range priorityCount {
		totalTests += count
	}

	if totalTests > 0 {
		criticalRatio := float64(priorityCount[PriorityCritical]) / float64(totalTests)
		if criticalRatio > 0.3 {
			warnings = append(warnings, "Coverage: Too many critical priority tests (>30%)")
		}
		if criticalRatio == 0 {
			warnings = append(warnings, "Coverage: No critical priority tests")
		}
	}

	return warnings
}

// ValidateAndFix attempts to fix common issues in a test suite
func (v *Validator) ValidateAndFix(suite *TestSuite) []string {
	var fixes []string

	// Generate missing IDs
	for i := range suite.Features {
		feature := &suite.Features[i]
		if feature.ID == "" {
			feature.ID = fmt.Sprintf("feature-%d", i+1)
			fixes = append(fixes, fmt.Sprintf("Generated ID for feature: %s", feature.Name))
		}

		for j := range feature.Scenarios {
			scenario := &feature.Scenarios[j]
			if scenario.ID == "" {
				scenario.ID = fmt.Sprintf("%s-scenario-%d", feature.ID, j+1)
				fixes = append(fixes, fmt.Sprintf("Generated ID for scenario: %s", scenario.Name))
			}

			for k := range scenario.TestCases {
				tc := &scenario.TestCases[k]
				if tc.ID == "" {
					tc.ID = fmt.Sprintf("%s-tc-%d", scenario.ID, k+1)
					fixes = append(fixes, fmt.Sprintf("Generated ID for test case: %s", tc.Name))
				}

				// Fix step ordering
				for l := range tc.Steps {
					if tc.Steps[l].Order == 0 {
						tc.Steps[l].Order = l + 1
					}
				}

				// Default priority if missing
				if tc.Priority == "" {
					tc.Priority = PriorityMedium
					fixes = append(fixes, fmt.Sprintf("Set default priority for: %s", tc.Name))
				}

				// Default type if missing
				if tc.Type == "" {
					tc.Type = TestTypeRegression
					fixes = append(fixes, fmt.Sprintf("Set default type for: %s", tc.Name))
				}

				// Default category if missing
				if tc.Category == "" {
					tc.Category = CategoryFunctional
					fixes = append(fixes, fmt.Sprintf("Set default category for: %s", tc.Name))
				}
			}
		}
	}

	return fixes
}

// GetValidationSummary returns a summary of validation results
func (v *Validator) GetValidationSummary(suite *TestSuite) map[string]interface{} {
	warnings := v.ValidateSuite(suite)

	// Count issues by type
	issueTypes := make(map[string]int)
	for _, w := range warnings {
		if strings.Contains(w, "Missing") {
			issueTypes["missing_fields"]++
		} else if strings.Contains(w, "Coverage") {
			issueTypes["coverage_gaps"]++
		} else if strings.Contains(w, "Selector") {
			issueTypes["selector_quality"]++
		} else if strings.Contains(w, "Duplicate") {
			issueTypes["duplicates"]++
		} else {
			issueTypes["other"]++
		}
	}

	return map[string]interface{}{
		"total_warnings": len(warnings),
		"warnings":       warnings,
		"issue_types":    issueTypes,
		"suite_valid":    len(warnings) == 0,
	}
}
