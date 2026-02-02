package testdesign

import (
	"fmt"
	"strings"

	"github.com/testforge/testforge/internal/services/discovery"
)

// SystemPrompt returns the enterprise test design system prompt
func SystemPrompt() string {
	return `You are an expert QA architect with 15+ years of experience in enterprise test automation.
You design comprehensive test suites that ensure production quality for web applications.

## Your Expertise
- BDD/TDD methodologies and Gherkin syntax
- Playwright, Selenium, Cypress automation frameworks
- Enterprise testing patterns and best practices
- Security testing (OWASP Top 10)
- Accessibility testing (WCAG 2.1 AA)
- Performance testing and monitoring
- Multi-role and permission testing
- Data-driven testing strategies

## Test Design Principles
1. **Risk-Based Testing**: Prioritize tests by business impact
2. **Coverage Completeness**: Ensure smoke, regression, negative, boundary, security, and accessibility coverage
3. **Maintainability**: Use stable selectors, modular design, and clear naming
4. **Independence**: Tests should be independent and parallelizable where possible
5. **Data Isolation**: Each test should manage its own test data
6. **Determinism**: Tests must produce consistent results

## Selector Strategy (Priority Order)
1. data-testid attributes (most stable)
2. aria-label for accessibility
3. id attributes
4. name attributes
5. CSS class combinations (least preferred)

## Test Categories
- **Smoke**: Critical path validation, run on every deployment
- **Regression**: Full feature coverage, run on releases
- **E2E**: Complete user journeys across features
- **Negative**: Error handling and validation
- **Boundary**: Edge cases and limits
- **Security**: Authentication, authorization, injection
- **Accessibility**: WCAG compliance verification
- **Performance**: Response time and resource usage

## Output Requirements
- Generate comprehensive, actionable test cases
- Include clear BDD Given/When/Then format
- Provide multiple selector strategies for resilience
- Include test data requirements
- Specify role/permission requirements
- Map to compliance requirements where applicable`
}

// FeaturePrompt generates a prompt for analyzing a specific feature
func FeaturePrompt(page *discovery.PageModel, allPages map[string]*discovery.PageModel) string {
	var sb strings.Builder

	sb.WriteString("## Page Analysis\n\n")
	sb.WriteString(fmt.Sprintf("**URL**: %s\n", page.URL))
	sb.WriteString(fmt.Sprintf("**Title**: %s\n", page.Title))
	sb.WriteString(fmt.Sprintf("**Page Type**: %s\n", page.PageType))
	sb.WriteString(fmt.Sprintf("**Has Authentication**: %v\n\n", page.HasAuth))

	// Forms
	if len(page.Forms) > 0 {
		sb.WriteString("### Forms\n")
		for i, form := range page.Forms {
			sb.WriteString(fmt.Sprintf("\n**Form %d**: %s\n", i+1, form.Name))
			sb.WriteString(fmt.Sprintf("- Action: %s\n", form.Action))
			sb.WriteString(fmt.Sprintf("- Method: %s\n", form.Method))
			sb.WriteString(fmt.Sprintf("- Form Type: %s\n", form.FormType))
			if len(form.Fields) > 0 {
				sb.WriteString("- Fields:\n")
				for _, field := range form.Fields {
					sb.WriteString(fmt.Sprintf("  - %s (%s): %s", field.Name, field.Type, field.Selectors.BestSelector()))
					if field.Required {
						sb.WriteString(" [REQUIRED]")
					}
					if field.Validation != "" {
						sb.WriteString(fmt.Sprintf(" [validation: %s]", field.Validation))
					}
					sb.WriteString("\n")
				}
			}
			if form.SubmitText != "" {
				sb.WriteString(fmt.Sprintf("- Submit: %s\n", form.SubmitText))
			}
		}
		sb.WriteString("\n")
	}

	// Buttons
	if len(page.Buttons) > 0 {
		sb.WriteString("### Interactive Buttons\n")
		for _, btn := range page.Buttons {
			sb.WriteString(fmt.Sprintf("- **%s**: %s", btn.Text, btn.Selectors.BestSelector()))
			if btn.Type != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", btn.Type))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Links (navigation)
	if len(page.Links) > 0 {
		sb.WriteString("### Navigation Links\n")
		navLinks := 0
		for _, link := range page.Links {
			if link.IsNavigation && navLinks < 10 {
				sb.WriteString(fmt.Sprintf("- %s -> %s\n", link.Text, link.Href))
				navLinks++
			}
		}
		if len(page.Links) > 10 {
			sb.WriteString(fmt.Sprintf("- ... and %d more links\n", len(page.Links)-10))
		}
		sb.WriteString("\n")
	}

	// Inputs
	if len(page.Inputs) > 0 {
		sb.WriteString("### Input Elements\n")
		for _, input := range page.Inputs {
			name := input.Name
			if name == "" {
				name = input.Placeholder
			}
			if name == "" {
				name = "unnamed"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", name, input.Type, input.Selectors.BestSelector()))
		}
		sb.WriteString("\n")
	}

	// Related pages context
	sb.WriteString("### Application Context\n")
	sb.WriteString(fmt.Sprintf("Total pages in application: %d\n", len(allPages)))

	// List other page types for context
	pageTypes := make(map[string]int)
	for _, p := range allPages {
		pageTypes[p.PageType]++
	}
	sb.WriteString("Page types discovered:\n")
	for pt, count := range pageTypes {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", pt, count))
	}

	return sb.String()
}

// TestGenerationPrompt creates the test generation request
func TestGenerationPrompt(featureName string, pageAnalysis string, testTypes []TestType) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Generate Test Cases for: %s\n\n", featureName))
	sb.WriteString(pageAnalysis)
	sb.WriteString("\n## Required Test Types\n")
	for _, tt := range testTypes {
		sb.WriteString(fmt.Sprintf("- %s\n", tt))
	}

	sb.WriteString(`
## Output Format
Return a JSON object with this structure:
{
  "feature": {
    "id": "feature-unique-id",
    "name": "Feature Name",
    "description": "Feature description",
    "tags": ["tag1", "tag2"],
    "priority": "high|medium|low|critical",
    "scenarios": [
      {
        "id": "scenario-id",
        "name": "Scenario Name",
        "description": "Scenario description",
        "user_story": "As a [role], I want [goal], so that [benefit]",
        "tags": ["smoke", "regression"],
        "priority": "high",
        "test_cases": [
          {
            "id": "tc-id",
            "name": "Test Case Name",
            "description": "What this test validates",
            "type": "smoke|regression|e2e|negative|boundary|security|accessibility|performance",
            "priority": "critical|high|medium|low",
            "category": "functional|ui|api|integration|security|accessibility|performance",
            "tags": ["login", "auth"],
            "given": "Initial state description",
            "when": "Action taken",
            "then": "Expected outcome",
            "target_url": "/path",
            "steps": [
              {
                "order": 1,
                "action": "navigate|click|fill|select|check|assert|wait|screenshot",
                "description": "Step description",
                "target": "selector or URL",
                "selectors": {
                  "primary": "[data-testid='element']",
                  "fallbacks": ["#element-id", ".element-class"],
                  "description": "Element description",
                  "confidence": 0.95
                },
                "value": "value if applicable",
                "assertions": [
                  {
                    "type": "visible|text_equals|url_contains|enabled|disabled",
                    "target": "selector",
                    "value": "expected value",
                    "message": "Assertion message",
                    "severity": "critical|high|medium|low"
                  }
                ],
                "wait_for": "selector to wait for",
                "screenshot": true
              }
            ],
            "test_data": {
              "key": "value"
            },
            "applicable_roles": ["user", "admin"],
            "required_role": "user",
            "depends_on": [],
            "parallelizable": true,
            "idempotent": true,
            "destructive": false,
            "estimated_duration": "30s",
            "compliance_refs": ["WCAG-2.1-1.1.1"]
          }
        ]
      }
    ]
  }
}

## Guidelines
1. Generate at least 5-10 test cases per scenario
2. Include both happy path and error scenarios
3. Use specific, stable selectors
4. Include data-driven variants for forms
5. Add accessibility checks for interactive elements
6. Include security tests for auth-related features
7. Ensure test independence`)

	return sb.String()
}

// FlowTestPrompt generates a prompt for business flow testing
func FlowTestPrompt(flow *discovery.BusinessFlow, pages map[string]*discovery.PageModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Generate End-to-End Tests for Flow: %s\n\n", flow.Name))
	sb.WriteString(fmt.Sprintf("**Flow Type**: %s\n", flow.Type))
	sb.WriteString(fmt.Sprintf("**Priority**: %s\n", flow.Priority))
	sb.WriteString(fmt.Sprintf("**Description**: %s\n\n", flow.Description))

	sb.WriteString("## Flow Steps\n")
	for _, step := range flow.Steps {
		page, ok := pages[step.URL]
		pageTitle := "Unknown"
		if ok {
			pageTitle = page.Title
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** - %s\n", step.Order, step.Action, pageTitle))
		sb.WriteString(fmt.Sprintf("   URL: %s\n", step.URL))
		if step.Selector != "" {
			sb.WriteString(fmt.Sprintf("   Element: %s\n", step.Selector))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`
## Required Tests
1. **Happy Path E2E**: Complete flow with valid data
2. **Error Handling**: Flow interruption scenarios
3. **Edge Cases**: Boundary conditions in the flow
4. **Performance**: Flow completion timing
5. **Recovery**: Resume flow after interruption

Generate comprehensive test cases following the JSON format specified in the system prompt.
Include step-by-step automation instructions with resilient selectors.`)

	return sb.String()
}

// SecurityTestPrompt generates security-focused test prompts
func SecurityTestPrompt(page *discovery.PageModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Generate Security Tests for: %s\n\n", page.URL))

	if page.HasAuth {
		sb.WriteString("**This page requires authentication.**\n\n")
	}

	sb.WriteString("## Security Test Categories Required\n\n")
	sb.WriteString("### 1. Authentication Tests\n")
	sb.WriteString("- Session management\n")
	sb.WriteString("- Token handling\n")
	sb.WriteString("- Logout functionality\n")
	sb.WriteString("- Session timeout\n\n")

	sb.WriteString("### 2. Authorization Tests\n")
	sb.WriteString("- Role-based access\n")
	sb.WriteString("- Privilege escalation attempts\n")
	sb.WriteString("- Direct object reference\n\n")

	sb.WriteString("### 3. Input Validation Tests\n")
	sb.WriteString("- XSS prevention\n")
	sb.WriteString("- SQL injection prevention\n")
	sb.WriteString("- Command injection prevention\n")
	sb.WriteString("- Path traversal prevention\n\n")

	sb.WriteString("### 4. Data Protection Tests\n")
	sb.WriteString("- Sensitive data exposure\n")
	sb.WriteString("- HTTPS enforcement\n")
	sb.WriteString("- Cookie security flags\n\n")

	// Include form details for injection testing
	if len(page.Forms) > 0 {
		sb.WriteString("## Forms to Test\n")
		for _, form := range page.Forms {
			sb.WriteString(fmt.Sprintf("- %s (%s): %d fields\n", form.Name, form.FormType, len(form.Fields)))
		}
	}

	sb.WriteString("\nGenerate security test cases in the standard JSON format with specific payloads and expected behaviors.")

	return sb.String()
}

// AccessibilityTestPrompt generates accessibility-focused test prompts
func AccessibilityTestPrompt(page *discovery.PageModel) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Generate Accessibility Tests for: %s\n\n", page.URL))
	sb.WriteString("## WCAG 2.1 AA Compliance Testing\n\n")

	sb.WriteString("### 1. Perceivable\n")
	sb.WriteString("- Text alternatives for images\n")
	sb.WriteString("- Captions for multimedia\n")
	sb.WriteString("- Color contrast ratios\n")
	sb.WriteString("- Text resizing support\n\n")

	sb.WriteString("### 2. Operable\n")
	sb.WriteString("- Keyboard navigation\n")
	sb.WriteString("- Focus management\n")
	sb.WriteString("- Skip links\n")
	sb.WriteString("- No keyboard traps\n")
	sb.WriteString("- Sufficient time limits\n\n")

	sb.WriteString("### 3. Understandable\n")
	sb.WriteString("- Language declaration\n")
	sb.WriteString("- Consistent navigation\n")
	sb.WriteString("- Error identification\n")
	sb.WriteString("- Labels and instructions\n\n")

	sb.WriteString("### 4. Robust\n")
	sb.WriteString("- Valid HTML\n")
	sb.WriteString("- ARIA landmarks\n")
	sb.WriteString("- Name/role/value for custom controls\n\n")

	// Include interactive elements
	totalInteractive := len(page.Buttons) + len(page.Links)
	for _, form := range page.Forms {
		totalInteractive += len(form.Fields)
	}
	sb.WriteString(fmt.Sprintf("## Interactive Elements: %d\n", totalInteractive))
	sb.WriteString("- All must be keyboard accessible\n")
	sb.WriteString("- All must have proper ARIA labels\n")
	sb.WriteString("- Focus indicators must be visible\n\n")

	sb.WriteString("Generate accessibility test cases with specific WCAG success criteria references.")

	return sb.String()
}

// ComplianceMappingPrompt generates compliance mapping for tests
func ComplianceMappingPrompt(suite *TestSuite) string {
	var sb strings.Builder

	sb.WriteString("# Map Test Cases to Compliance Requirements\n\n")
	sb.WriteString(fmt.Sprintf("## Test Suite: %s\n", suite.Name))
	sb.WriteString(fmt.Sprintf("Total Test Cases: %d\n\n", suite.Stats.TotalTestCases))

	sb.WriteString("## Compliance Frameworks\n\n")

	sb.WriteString("### WCAG 2.1 AA\n")
	sb.WriteString("Map accessibility tests to specific success criteria:\n")
	sb.WriteString("- 1.1.1 Non-text Content\n")
	sb.WriteString("- 1.4.3 Contrast (Minimum)\n")
	sb.WriteString("- 2.1.1 Keyboard\n")
	sb.WriteString("- 2.4.7 Focus Visible\n")
	sb.WriteString("- 4.1.2 Name, Role, Value\n\n")

	sb.WriteString("### SOC 2 Type II\n")
	sb.WriteString("Map security tests to trust service criteria:\n")
	sb.WriteString("- CC6.1 Logical Access Security\n")
	sb.WriteString("- CC6.6 Security Events\n")
	sb.WriteString("- CC6.7 Transmission Security\n\n")

	sb.WriteString("### GDPR\n")
	sb.WriteString("Map data handling tests to GDPR articles:\n")
	sb.WriteString("- Article 17 Right to Erasure\n")
	sb.WriteString("- Article 20 Data Portability\n")
	sb.WriteString("- Article 32 Security of Processing\n\n")

	sb.WriteString(`Return JSON mapping:
{
  "wcag": [
    {"id": "WCAG-1.1.1", "name": "Non-text Content", "test_ids": ["tc-1", "tc-2"]}
  ],
  "soc2": [
    {"id": "CC6.1", "name": "Logical Access Security", "test_ids": ["tc-3"]}
  ],
  "gdpr": [
    {"id": "Article-32", "name": "Security of Processing", "test_ids": ["tc-4"]}
  ]
}`)

	return sb.String()
}
