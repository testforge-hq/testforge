package scriptgen

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/testforge/testforge/internal/services/testdesign"
)

// ScriptGenerator converts TestSuite into Playwright scripts
type ScriptGenerator struct {
	config    GeneratorConfig
	templates map[string]*template.Template
}

// NewScriptGenerator creates a new script generator
func NewScriptGenerator(config GeneratorConfig) *ScriptGenerator {
	g := &ScriptGenerator{
		config:    config,
		templates: make(map[string]*template.Template),
	}
	g.initTemplates()
	return g
}

// initTemplates parses all templates
func (g *ScriptGenerator) initTemplates() {
	// Templates are mostly static, we'll use string interpolation
}

// GenerateScripts converts a TestSuite into a complete Playwright project
func (g *ScriptGenerator) GenerateScripts(suite *testdesign.TestSuite) (*GeneratedProject, error) {
	project := &GeneratedProject{
		Files: make(map[string]string),
		Metadata: ProjectMetadata{
			GeneratedAt:    time.Now(),
			SuiteID:        suite.ID,
			SuiteName:      suite.Name,
			TotalFeatures:  suite.Stats.TotalFeatures,
			TotalScenarios: suite.Stats.TotalScenarios,
			TotalTests:     suite.Stats.TotalTestCases,
			TotalSteps:     suite.Stats.TotalSteps,
		},
	}

	// 1. Generate config files
	project.Files["playwright.config.ts"] = g.renderPlaywrightConfig(suite)
	project.Files["package.json"] = g.renderPackageJSON(suite)
	project.Files["tsconfig.json"] = TSConfigTemplate
	project.Files[".env.example"] = g.renderEnvExample(suite)
	project.Files[".gitignore"] = GitignoreTemplate
	project.Files["README.md"] = g.renderReadme(suite)

	// 2. Generate base utilities
	project.Files["pages/base.page.ts"] = BasePageTemplate
	project.Files["utils/selectors.ts"] = SelectorsUtilTemplate
	project.Files["utils/assertions.ts"] = AssertionsUtilTemplate

	// 3. Generate fixtures
	project.Files["fixtures/auth.fixture.ts"] = AuthFixtureTemplate
	project.Files["fixtures/test-data.ts"] = g.renderTestData(suite)

	// 4. Generate Page Objects
	pages := g.extractUniquePages(suite)
	for _, page := range pages {
		filename := fmt.Sprintf("pages/%s.page.ts", page.Slug)
		project.Files[filename] = g.generatePageObject(page)
	}

	// 5. Generate test files by type
	testFiles := g.generateTestFiles(suite)
	for filename, content := range testFiles {
		project.Files[filename] = content
	}

	// 6. Create empty directories
	project.Files["reports/.gitkeep"] = ""
	project.Files["auth/.gitkeep"] = ""

	// Calculate statistics
	project.TestCount = suite.Stats.TotalTestCases
	project.PageCount = len(pages)
	project.LinesOfCode = g.countLOC(project.Files)

	return project, nil
}

// renderPlaywrightConfig generates playwright.config.ts
func (g *ScriptGenerator) renderPlaywrightConfig(suite *testdesign.TestSuite) string {
	content := PlaywrightConfigTemplate
	content = strings.ReplaceAll(content, "{{.BaseURL}}", g.config.BaseURL)
	return content
}

// renderPackageJSON generates package.json
func (g *ScriptGenerator) renderPackageJSON(suite *testdesign.TestSuite) string {
	content := PackageJSONTemplate
	content = strings.ReplaceAll(content, "{{.ProjectName}}", g.slugify(suite.Name))
	content = strings.ReplaceAll(content, "{{.GeneratedAt}}", time.Now().Format(time.RFC3339))
	content = strings.ReplaceAll(content, "{{.SuiteID}}", suite.ID)
	return content
}

// renderEnvExample generates .env.example
func (g *ScriptGenerator) renderEnvExample(suite *testdesign.TestSuite) string {
	content := EnvExampleTemplate
	content = strings.ReplaceAll(content, "{{.BaseURL}}", g.config.BaseURL)
	content = strings.ReplaceAll(content, "{{.APIEndpoint}}", g.config.APIEndpoint)
	content = strings.ReplaceAll(content, "{{.TestRunID}}", g.config.TestRunID)
	content = strings.ReplaceAll(content, "{{.TenantID}}", g.config.TenantID)
	return content
}

// renderTestData generates fixtures/test-data.ts
func (g *ScriptGenerator) renderTestData(suite *testdesign.TestSuite) string {
	content := TestDataTemplate
	content = strings.ReplaceAll(content, "{{.BaseURL}}", g.config.BaseURL)
	return content
}

// renderReadme generates README.md
func (g *ScriptGenerator) renderReadme(suite *testdesign.TestSuite) string {
	content := ReadmeTemplate
	content = strings.ReplaceAll(content, "{{.ProjectName}}", suite.Name)
	content = strings.ReplaceAll(content, "{{.GeneratedAt}}", time.Now().Format("2006-01-02 15:04:05"))
	content = strings.ReplaceAll(content, "{{.TotalFeatures}}", fmt.Sprintf("%d", suite.Stats.TotalFeatures))
	content = strings.ReplaceAll(content, "{{.TotalScenarios}}", fmt.Sprintf("%d", suite.Stats.TotalScenarios))
	content = strings.ReplaceAll(content, "{{.TotalTests}}", fmt.Sprintf("%d", suite.Stats.TotalTestCases))
	content = strings.ReplaceAll(content, "{{.TotalSteps}}", fmt.Sprintf("%d", suite.Stats.TotalSteps))
	return content
}

// extractUniquePages extracts unique pages from the test suite for POM generation
func (g *ScriptGenerator) extractUniquePages(suite *testdesign.TestSuite) []PageObjectInfo {
	pageMap := make(map[string]*PageObjectInfo)

	for _, feature := range suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				url := tc.TargetURL
				if url == "" {
					continue
				}

				slug := g.urlToSlug(url)
				if _, exists := pageMap[slug]; !exists {
					pageMap[slug] = &PageObjectInfo{
						Name:        g.urlToClassName(url),
						Slug:        slug,
						URL:         url,
						Title:       tc.Name,
						Description: tc.Description,
						Selectors:   []SelectorInfo{},
						Actions:     []ActionInfo{},
					}
				}

				// Extract selectors from steps
				page := pageMap[slug]
				for _, step := range tc.Steps {
					if step.Selectors != nil && step.Selectors.Primary != "" {
						sel := SelectorInfo{
							Name:        g.selectorToName(step.Selectors.Primary),
							Description: step.Selectors.Description,
							Primary:     step.Selectors.Primary,
							Fallbacks:   step.Selectors.Fallbacks,
							Confidence:  step.Selectors.Confidence,
							Type:        string(step.Action),
						}
						page.Selectors = append(page.Selectors, sel)
					}
				}
			}
		}
	}

	// Convert map to slice
	pages := make([]PageObjectInfo, 0, len(pageMap))
	for _, page := range pageMap {
		pages = append(pages, *page)
	}

	return pages
}

// generatePageObject generates a Page Object class
func (g *ScriptGenerator) generatePageObject(page PageObjectInfo) string {
	var buf bytes.Buffer

	// Imports
	buf.WriteString(`import { Page, Locator } from '@playwright/test';
import { BasePage } from './base.page';
import { ResilientSelector } from '../utils/selectors';

`)

	// Class documentation
	buf.WriteString(fmt.Sprintf(`/**
 * Page Object for: %s
 * URL: %s
 * Generated by TestForge
 */
`, page.Name, page.URL))

	// Class definition
	buf.WriteString(fmt.Sprintf("export class %sPage extends BasePage {\n", page.Name))

	// URL property
	buf.WriteString(fmt.Sprintf("  readonly url = '%s';\n\n", page.URL))

	// Generate selectors as properties
	selectorsSeen := make(map[string]bool)
	for _, sel := range page.Selectors {
		if selectorsSeen[sel.Name] {
			continue
		}
		selectorsSeen[sel.Name] = true

		fallbacks := "[]"
		if len(sel.Fallbacks) > 0 {
			fallbackStrs := make([]string, len(sel.Fallbacks))
			for i, f := range sel.Fallbacks {
				fallbackStrs[i] = fmt.Sprintf("'%s'", g.escapeString(f))
			}
			fallbacks = fmt.Sprintf("[%s]", strings.Join(fallbackStrs, ", "))
		}

		buf.WriteString(fmt.Sprintf("  /** %s */\n", sel.Description))
		buf.WriteString(fmt.Sprintf("  private readonly %sSelector = new ResilientSelector(\n", sel.Name))
		buf.WriteString(fmt.Sprintf("    '%s',\n", g.escapeString(sel.Primary)))
		buf.WriteString(fmt.Sprintf("    %s,\n", fallbacks))
		buf.WriteString(fmt.Sprintf("    '%s',\n", g.escapeString(sel.Description)))
		buf.WriteString(fmt.Sprintf("    %.2f\n", sel.Confidence))
		buf.WriteString("  );\n\n")
	}

	// Generate getter for each element
	for name := range selectorsSeen {
		buf.WriteString(fmt.Sprintf("  /** Get %s element */\n", name))
		buf.WriteString(fmt.Sprintf("  get %s(): Promise<Locator> {\n", name))
		buf.WriteString(fmt.Sprintf("    return this.%sSelector.locate(this.page);\n", name))
		buf.WriteString("  }\n\n")
	}

	// Generate common action methods based on page type
	buf.WriteString("  /** Navigate to this page and wait for load */\n")
	buf.WriteString("  async goto(): Promise<void> {\n")
	buf.WriteString("    await this.navigate();\n")
	buf.WriteString("  }\n\n")

	// If it looks like a form page, add form methods
	hasForm := false
	for _, sel := range page.Selectors {
		if sel.Type == "fill" || strings.Contains(strings.ToLower(sel.Primary), "input") {
			hasForm = true
			break
		}
	}

	if hasForm {
		buf.WriteString("  /** Submit the form on this page */\n")
		buf.WriteString("  async submit(): Promise<void> {\n")
		buf.WriteString("    await this.page.locator('button[type=\"submit\"], input[type=\"submit\"]').first().click();\n")
		buf.WriteString("    await this.waitForPageLoad();\n")
		buf.WriteString("  }\n\n")
	}

	buf.WriteString("}\n")

	return buf.String()
}

// generateTestFiles generates test files organized by type
func (g *ScriptGenerator) generateTestFiles(suite *testdesign.TestSuite) map[string]string {
	files := make(map[string]string)

	// Group tests by type
	testsByType := make(map[string][]testCaseWithContext)

	for _, feature := range suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				testType := string(tc.Type)
				if testType == "" {
					testType = "regression"
				}

				testsByType[testType] = append(testsByType[testType], testCaseWithContext{
					TestCase: tc,
					Feature:  feature,
					Scenario: scenario,
				})
			}
		}
	}

	// Generate file for each test type
	for testType, tests := range testsByType {
		dir := g.testTypeToDir(testType)
		filename := fmt.Sprintf("tests/%s/%s.spec.ts", dir, g.slugify(suite.Name))
		files[filename] = g.generateTestFile(testType, tests)
	}

	return files
}

type testCaseWithContext struct {
	TestCase testdesign.TestCase
	Feature  testdesign.Feature
	Scenario testdesign.Scenario
}

// generateTestFile generates a single test file
func (g *ScriptGenerator) generateTestFile(testType string, tests []testCaseWithContext) string {
	var buf bytes.Buffer

	// Imports - tests are in tests/<type>/ so need ../../ to reach root
	buf.WriteString(`import { test, expect } from '../../fixtures/auth.fixture';
import { ResilientSelector } from '../../utils/selectors';
import { Assertions } from '../../utils/assertions';

`)

	// Deduplicate tests - track seen test IDs and make them unique
	seenTests := make(map[string]int)
	for i := range tests {
		key := tests[i].Feature.ID + "-" + tests[i].Scenario.ID + "-" + tests[i].TestCase.ID
		if count, exists := seenTests[key]; exists {
			// Make the ID unique by appending a counter
			tests[i].TestCase.ID = fmt.Sprintf("%s-%d", tests[i].TestCase.ID, count+1)
		}
		seenTests[key]++
	}

	// Group by feature
	byFeature := make(map[string][]testCaseWithContext)
	for _, t := range tests {
		byFeature[t.Feature.Name] = append(byFeature[t.Feature.Name], t)
	}

	for featureName, featureTests := range byFeature {
		// Feature describe block
		buf.WriteString(fmt.Sprintf("/**\n * @feature %s\n * @type %s\n */\n", featureName, testType))
		buf.WriteString(fmt.Sprintf("test.describe('%s - %s Tests', () => {\n", featureName, strings.Title(testType)))

		// Group by scenario
		byScenario := make(map[string][]testCaseWithContext)
		for _, t := range featureTests {
			byScenario[t.Scenario.Name] = append(byScenario[t.Scenario.Name], t)
		}

		for scenarioName, scenarioTests := range byScenario {
			buf.WriteString(fmt.Sprintf("\n  test.describe('%s', () => {\n", scenarioName))

			for _, t := range scenarioTests {
				tc := t.TestCase

				// Test documentation
				buf.WriteString(fmt.Sprintf("    /**\n"))
				buf.WriteString(fmt.Sprintf("     * @testId %s\n", tc.ID))
				buf.WriteString(fmt.Sprintf("     * @priority %s\n", tc.Priority))
				if len(tc.ComplianceRefs) > 0 {
					buf.WriteString(fmt.Sprintf("     * @compliance %s\n", strings.Join(tc.ComplianceRefs, ", ")))
				}
				buf.WriteString(fmt.Sprintf("     */\n"))

				// Test tags
				tags := []string{fmt.Sprintf("@%s", testType)}
				if tc.Priority == testdesign.PriorityCritical {
					tags = append(tags, "@critical")
				}
				tagStr := strings.Join(tags, " ")

				// Fixture type based on role
				fixture := "guestPage"
				if tc.RequiredRole == "user" {
					fixture = "authenticatedPage"
				} else if tc.RequiredRole == "admin" {
					fixture = "adminPage"
				}

				// Include test ID in title to ensure uniqueness
				testTitle := tc.Name
				if tc.ID != "" {
					testTitle = fmt.Sprintf("[%s] %s", tc.ID, tc.Name)
				}

				buf.WriteString(fmt.Sprintf("    test('%s %s', async ({ %s }) => {\n", testTitle, tagStr, fixture))

				// BDD comments
				if tc.Given != "" {
					buf.WriteString(fmt.Sprintf("      // Given: %s\n", tc.Given))
				}
				if tc.When != "" {
					buf.WriteString(fmt.Sprintf("      // When: %s\n", tc.When))
				}
				if tc.Then != "" {
					buf.WriteString(fmt.Sprintf("      // Then: %s\n", tc.Then))
				}
				buf.WriteString("\n")

				// Generate steps
				for _, step := range tc.Steps {
					g.generateStep(&buf, step, fixture)
				}

				buf.WriteString("    });\n\n")
			}

			buf.WriteString("  });\n")
		}

		buf.WriteString("});\n\n")
	}

	return buf.String()
}

// generateStep generates code for a single test step
func (g *ScriptGenerator) generateStep(buf *bytes.Buffer, step testdesign.TestStep, pageName string) {
	indent := "      "

	// Comment for the step
	if step.Description != "" {
		buf.WriteString(fmt.Sprintf("%s// Step %d: %s\n", indent, step.Order, step.Description))
	}

	// Get selector
	selector := step.Target
	if step.Selectors != nil && step.Selectors.Primary != "" {
		selector = step.Selectors.Primary
	}

	switch step.Action {
	case testdesign.ActionNavigate:
		url := step.Value
		if url == "" {
			url = step.Target
		}
		buf.WriteString(fmt.Sprintf("%sawait %s.goto('%s');\n", indent, pageName, g.escapeString(url)))
		buf.WriteString(fmt.Sprintf("%sawait %s.waitForLoadState('networkidle');\n", indent, pageName))

	case testdesign.ActionClick:
		if selector != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').click();\n", indent, pageName, g.escapeString(selector)))
		}

	case testdesign.ActionFill:
		if selector != "" && step.Value != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').fill('%s');\n", indent, pageName, g.escapeString(selector), g.escapeString(step.Value)))
		}

	case testdesign.ActionSelect:
		if selector != "" && step.Value != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').selectOption('%s');\n", indent, pageName, g.escapeString(selector), g.escapeString(step.Value)))
		}

	case testdesign.ActionCheck:
		if selector != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').check();\n", indent, pageName, g.escapeString(selector)))
		}

	case testdesign.ActionUncheck:
		if selector != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').uncheck();\n", indent, pageName, g.escapeString(selector)))
		}

	case testdesign.ActionHover:
		if selector != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').hover();\n", indent, pageName, g.escapeString(selector)))
		}

	case testdesign.ActionWait:
		if step.WaitFor != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').waitFor({ state: 'visible' });\n", indent, pageName, g.escapeString(step.WaitFor)))
		} else if step.WaitTimeout != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.waitForTimeout(1000);\n", indent, pageName))
		}

	case testdesign.ActionAssert:
		g.generateAssertions(buf, step, pageName, indent)

	case testdesign.ActionScreenshot:
		name := step.ScreenshotName
		if name == "" {
			name = fmt.Sprintf("step_%d", step.Order)
		}
		buf.WriteString(fmt.Sprintf("%sawait %s.screenshot({ path: 'reports/screenshots/%s.png' });\n", indent, pageName, name))

	case testdesign.ActionKeyPress:
		if step.Value != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.keyboard.press('%s');\n", indent, pageName, step.Value))
		}

	case testdesign.ActionScroll:
		if selector != "" {
			buf.WriteString(fmt.Sprintf("%sawait %s.locator('%s').scrollIntoViewIfNeeded();\n", indent, pageName, g.escapeString(selector)))
		} else {
			buf.WriteString(fmt.Sprintf("%sawait %s.evaluate(() => window.scrollBy(0, 500));\n", indent, pageName))
		}

	default:
		buf.WriteString(fmt.Sprintf("%s// TODO: Implement action '%s'\n", indent, step.Action))
	}

	// Add step-level assertions
	if len(step.Assertions) > 0 && step.Action != testdesign.ActionAssert {
		g.generateAssertions(buf, step, pageName, indent)
	}

	buf.WriteString("\n")
}

// generateAssertions generates assertion code
func (g *ScriptGenerator) generateAssertions(buf *bytes.Buffer, step testdesign.TestStep, pageName string, indent string) {
	for _, assertion := range step.Assertions {
		target := assertion.Target
		if target == "" && step.Selectors != nil {
			target = step.Selectors.Primary
		}

		switch assertion.Type {
		case testdesign.AssertVisible:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeVisible();\n", indent, pageName, g.escapeString(target)))

		case testdesign.AssertHidden:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeHidden();\n", indent, pageName, g.escapeString(target)))

		case testdesign.AssertTextEquals:
			// Handle title element specially
			if target == "title" {
				buf.WriteString(fmt.Sprintf("%sawait expect(%s).toHaveTitle('%s');\n", indent, pageName, g.escapeString(assertion.Value)))
			} else {
				buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toHaveText('%s');\n", indent, pageName, g.escapeString(target), g.escapeString(assertion.Value)))
			}

		case testdesign.AssertTextContains:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toContainText('%s');\n", indent, pageName, g.escapeString(target), g.escapeString(assertion.Value)))

		case testdesign.AssertURLEquals:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s).toHaveURL('%s');\n", indent, pageName, g.escapeString(assertion.Value)))

		case testdesign.AssertURLContains:
			// Escape forward slashes for regex
			regexValue := strings.ReplaceAll(assertion.Value, "/", "\\/")
			buf.WriteString(fmt.Sprintf("%sawait expect(%s).toHaveURL(/%s/);\n", indent, pageName, regexValue))

		case testdesign.AssertTitleEquals:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s).toHaveTitle('%s');\n", indent, pageName, g.escapeString(assertion.Value)))

		case testdesign.AssertTitleContains:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s).toHaveTitle(/%s/);\n", indent, pageName, g.escapeString(assertion.Value)))

		case testdesign.AssertEnabled:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeEnabled();\n", indent, pageName, g.escapeString(target)))

		case testdesign.AssertDisabled:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeDisabled();\n", indent, pageName, g.escapeString(target)))

		case testdesign.AssertChecked:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeChecked();\n", indent, pageName, g.escapeString(target)))

		case testdesign.AssertValueEquals:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toHaveValue('%s');\n", indent, pageName, g.escapeString(target), g.escapeString(assertion.Value)))

		case testdesign.AssertElementCount:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toHaveCount(%s);\n", indent, pageName, g.escapeString(target), assertion.Value))

		case testdesign.AssertAttribute:
			parts := strings.SplitN(assertion.Value, "=", 2)
			if len(parts) == 2 {
				buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toHaveAttribute('%s', '%s');\n", indent, pageName, g.escapeString(target), parts[0], g.escapeString(parts[1])))
			}

		case testdesign.AssertFocused:
			buf.WriteString(fmt.Sprintf("%sawait expect(%s.locator('%s')).toBeFocused();\n", indent, pageName, g.escapeString(target)))

		default:
			buf.WriteString(fmt.Sprintf("%s// TODO: Implement assertion '%s'\n", indent, assertion.Type))
		}
	}
}

// Helper functions

func (g *ScriptGenerator) slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func (g *ScriptGenerator) urlToSlug(url string) string {
	// Remove protocol and domain
	url = regexp.MustCompile(`^https?://[^/]+`).ReplaceAllString(url, "")
	if url == "" || url == "/" {
		return "home"
	}
	return g.slugify(url)
}

func (g *ScriptGenerator) urlToClassName(url string) string {
	slug := g.urlToSlug(url)
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	name := strings.Join(parts, "")
	if name == "" {
		name = "Home"
	}
	return name
}

func (g *ScriptGenerator) selectorToName(selector string) string {
	// Extract meaningful name from selector
	name := selector

	// Try to extract from data-testid
	if matches := regexp.MustCompile(`data-testid="([^"]+)"`).FindStringSubmatch(selector); len(matches) > 1 {
		name = matches[1]
	} else if matches := regexp.MustCompile(`#([a-zA-Z][a-zA-Z0-9_-]*)`).FindStringSubmatch(selector); len(matches) > 1 {
		name = matches[1]
	} else if matches := regexp.MustCompile(`\[name="([^"]+)"\]`).FindStringSubmatch(selector); len(matches) > 1 {
		name = matches[1]
	}

	// Convert to camelCase
	name = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(name, "_")
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(part)
		} else if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}

	return strings.Join(parts, "")
}

func (g *ScriptGenerator) testTypeToDir(testType string) string {
	switch testType {
	case "smoke":
		return "smoke"
	case "regression":
		return "regression"
	case "e2e":
		return "e2e"
	case "accessibility":
		return "accessibility"
	case "security":
		return "security"
	case "performance":
		return "performance"
	case "negative", "boundary":
		return "regression"
	default:
		return "regression"
	}
}

func (g *ScriptGenerator) escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func (g *ScriptGenerator) countLOC(files map[string]string) int {
	total := 0
	for _, content := range files {
		total += strings.Count(content, "\n") + 1
	}
	return total
}
