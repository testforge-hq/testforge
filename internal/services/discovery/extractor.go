package discovery

import (
	"strings"

	"github.com/playwright-community/playwright-go"
)

// PageElements contains all extracted elements from a page
type PageElements struct {
	Forms      []FormModel
	Buttons    []ButtonModel
	Links      []LinkModel
	Inputs     []InputModel
	Navigation []NavItem
	Title      string
	MetaDesc   string
	HasAuth    bool
	PageType   string
}

// Extractor extracts interactive elements from a page
type Extractor struct{}

// NewExtractor creates a new element extractor
func NewExtractor() *Extractor {
	return &Extractor{}
}

// ExtractPageElements extracts all interactive elements from a page
func (e *Extractor) ExtractPageElements(page playwright.Page, baseURL string) (*PageElements, error) {
	elements := &PageElements{}

	// Extract title and meta description
	title, _ := page.Title()
	elements.Title = title

	metaDesc, _ := page.Locator(`meta[name="description"]`).GetAttribute("content")
	elements.MetaDesc = metaDesc

	// Extract forms
	forms, err := e.extractForms(page)
	if err == nil {
		elements.Forms = forms
	}

	// Extract buttons
	buttons, err := e.extractButtons(page)
	if err == nil {
		elements.Buttons = buttons
	}

	// Extract links
	links, err := e.extractLinks(page, baseURL)
	if err == nil {
		elements.Links = links
	}

	// Extract standalone inputs
	inputs, err := e.extractInputs(page)
	if err == nil {
		elements.Inputs = inputs
	}

	// Extract navigation
	nav, err := e.extractNavigation(page)
	if err == nil {
		elements.Navigation = nav
	}

	// Detect if page has authentication
	elements.HasAuth = e.detectAuth(page)

	// Classify page type
	elements.PageType = e.classifyPage(elements)

	return elements, nil
}

// extractForms extracts all forms from the page
func (e *Extractor) extractForms(page playwright.Page) ([]FormModel, error) {
	var forms []FormModel

	formLocators := page.Locator("form")
	count, err := formLocators.Count()
	if err != nil {
		return forms, err
	}

	for i := 0; i < count; i++ {
		form := formLocators.Nth(i)

		formModel := FormModel{
			Method: "GET",
		}

		// Get form attributes
		if id, err := form.GetAttribute("id"); err == nil {
			formModel.ID = id
		}
		if name, err := form.GetAttribute("name"); err == nil {
			formModel.Name = name
		}
		if action, err := form.GetAttribute("action"); err == nil {
			formModel.Action = action
		}
		if method, err := form.GetAttribute("method"); err == nil {
			formModel.Method = strings.ToUpper(method)
		}

		// Build selectors
		formModel.Selectors = e.buildSelectors(form)

		// Extract form fields
		fields, _ := e.extractFormFields(form)
		formModel.Fields = fields

		// Get submit button text
		submitBtn := form.Locator(`button[type="submit"], input[type="submit"]`).First()
		if submitText, err := submitBtn.TextContent(); err == nil {
			formModel.SubmitText = strings.TrimSpace(submitText)
		} else if submitValue, err := submitBtn.GetAttribute("value"); err == nil {
			formModel.SubmitText = submitValue
		}

		// Classify form type
		formModel.FormType = e.classifyFormType(formModel)

		forms = append(forms, formModel)
	}

	return forms, nil
}

// extractFormFields extracts fields from a form
func (e *Extractor) extractFormFields(form playwright.Locator) ([]FieldModel, error) {
	var fields []FieldModel

	// Extract input fields
	inputLocators := form.Locator("input:not([type='hidden']):not([type='submit']):not([type='button']), textarea, select")
	count, err := inputLocators.Count()
	if err != nil {
		return fields, err
	}

	for i := 0; i < count; i++ {
		input := inputLocators.Nth(i)

		field := FieldModel{
			Type: "text",
		}

		if inputType, err := input.GetAttribute("type"); err == nil && inputType != "" {
			field.Type = inputType
		}
		if name, err := input.GetAttribute("name"); err == nil {
			field.Name = name
		}
		if placeholder, err := input.GetAttribute("placeholder"); err == nil {
			field.Placeholder = placeholder
		}
		if _, err := input.GetAttribute("required"); err == nil {
			field.Required = true
		}

		// Try to find associated label
		if id, err := input.GetAttribute("id"); err == nil && id != "" {
			label := form.Locator(`label[for="` + id + `"]`)
			if labelText, err := label.TextContent(); err == nil {
				field.Label = strings.TrimSpace(labelText)
			}
		}

		field.Selectors = e.buildSelectors(input)
		field.Validation = e.detectFieldValidation(field)

		fields = append(fields, field)
	}

	return fields, nil
}

// extractButtons extracts all buttons from the page
func (e *Extractor) extractButtons(page playwright.Page) ([]ButtonModel, error) {
	var buttons []ButtonModel

	// Query for buttons including role="button"
	buttonLocators := page.Locator(`button, input[type="button"], input[type="submit"], [role="button"]`)
	count, err := buttonLocators.Count()
	if err != nil {
		return buttons, err
	}

	for i := 0; i < count; i++ {
		btn := buttonLocators.Nth(i)

		button := ButtonModel{
			Type: "button",
		}

		if text, err := btn.TextContent(); err == nil {
			button.Text = strings.TrimSpace(text)
		}
		if btnType, err := btn.GetAttribute("type"); err == nil {
			button.Type = btnType
		}
		if ariaLabel, err := btn.GetAttribute("aria-label"); err == nil {
			button.AriaLabel = ariaLabel
		}
		if _, err := btn.GetAttribute("disabled"); err == nil {
			button.Disabled = true
		}
		if onClick, err := btn.GetAttribute("onclick"); err == nil {
			button.OnClick = onClick
		}

		button.Selectors = e.buildSelectors(btn)

		buttons = append(buttons, button)
	}

	return buttons, nil
}

// extractLinks extracts all links from the page
func (e *Extractor) extractLinks(page playwright.Page, baseURL string) ([]LinkModel, error) {
	var links []LinkModel

	linkLocators := page.Locator("a[href]")
	count, err := linkLocators.Count()
	if err != nil {
		return links, err
	}

	baseDomain := extractDomain(baseURL)

	for i := 0; i < count; i++ {
		link := linkLocators.Nth(i)

		linkModel := LinkModel{}

		if text, err := link.TextContent(); err == nil {
			linkModel.Text = strings.TrimSpace(text)
		}
		if href, err := link.GetAttribute("href"); err == nil {
			linkModel.Href = href
			linkModel.IsInternal = isInternalLink(href, baseDomain)
		}

		// Check if link is part of navigation
		parent := link.Locator("xpath=ancestor::nav | ancestor::*[contains(@class, 'nav')] | ancestor::header")
		if navCount, _ := parent.Count(); navCount > 0 {
			linkModel.IsNavigation = true
		}

		linkModel.Selectors = e.buildSelectors(link)

		links = append(links, linkModel)
	}

	return links, nil
}

// extractInputs extracts standalone input elements (not in forms)
func (e *Extractor) extractInputs(page playwright.Page) ([]InputModel, error) {
	var inputs []InputModel

	// Find inputs not inside forms
	inputLocators := page.Locator("input:not(form input):not([type='hidden']):not([type='submit']):not([type='button'])")
	count, err := inputLocators.Count()
	if err != nil {
		return inputs, err
	}

	for i := 0; i < count; i++ {
		input := inputLocators.Nth(i)

		inputModel := InputModel{
			Type: "text",
		}

		if inputType, err := input.GetAttribute("type"); err == nil && inputType != "" {
			inputModel.Type = inputType
		}
		if name, err := input.GetAttribute("name"); err == nil {
			inputModel.Name = name
		}
		if placeholder, err := input.GetAttribute("placeholder"); err == nil {
			inputModel.Placeholder = placeholder
		}
		if ariaLabel, err := input.GetAttribute("aria-label"); err == nil {
			inputModel.AriaLabel = ariaLabel
		}

		inputModel.Selectors = e.buildSelectors(input)

		inputs = append(inputs, inputModel)
	}

	return inputs, nil
}

// extractNavigation extracts navigation structure
func (e *Extractor) extractNavigation(page playwright.Page) ([]NavItem, error) {
	var navItems []NavItem

	// Look for nav elements
	navLocators := page.Locator("nav a, header a, [role='navigation'] a")
	count, err := navLocators.Count()
	if err != nil {
		return navItems, err
	}

	for i := 0; i < count; i++ {
		navLink := navLocators.Nth(i)

		item := NavItem{}

		if text, err := navLink.TextContent(); err == nil {
			item.Text = strings.TrimSpace(text)
		}
		if href, err := navLink.GetAttribute("href"); err == nil {
			item.Href = href
		}

		if item.Text != "" && item.Href != "" {
			navItems = append(navItems, item)
		}
	}

	return navItems, nil
}

// buildSelectors builds selector candidates for an element
func (e *Extractor) buildSelectors(locator playwright.Locator) SelectorCandidates {
	selectors := SelectorCandidates{}

	// data-testid (best)
	if testID, err := locator.GetAttribute("data-testid"); err == nil && testID != "" {
		selectors.TestID = testID
	}

	// data-cy or data-test (Cypress conventions)
	if dataCy, err := locator.GetAttribute("data-cy"); err == nil && dataCy != "" {
		selectors.CypressID = dataCy
	} else if dataTest, err := locator.GetAttribute("data-test"); err == nil && dataTest != "" {
		selectors.CypressID = dataTest
	}

	// id attribute
	if id, err := locator.GetAttribute("id"); err == nil && id != "" {
		selectors.ID = id
	}

	// aria-label
	if ariaLabel, err := locator.GetAttribute("aria-label"); err == nil && ariaLabel != "" {
		selectors.AriaLabel = ariaLabel
	}

	// name attribute
	if name, err := locator.GetAttribute("name"); err == nil && name != "" {
		selectors.Name = name
	}

	// Text content for buttons/links
	if text, err := locator.TextContent(); err == nil {
		text = strings.TrimSpace(text)
		if len(text) > 0 && len(text) < 50 {
			selectors.TextContent = text
		}
	}

	// Build CSS selector as fallback
	selectors.CSS = e.buildCSSSelector(locator)

	return selectors
}

// buildCSSSelector builds a CSS selector for an element
func (e *Extractor) buildCSSSelector(locator playwright.Locator) string {
	// Try to build a unique CSS selector
	// This is a simplified version - in production would be more robust

	tagName, _ := locator.Evaluate("el => el.tagName.toLowerCase()", nil)
	if tag, ok := tagName.(string); ok {
		if id, err := locator.GetAttribute("id"); err == nil && id != "" {
			return tag + "#" + id
		}
		if class, err := locator.GetAttribute("class"); err == nil && class != "" {
			classes := strings.Fields(class)
			if len(classes) > 0 {
				return tag + "." + strings.Join(classes[:min(3, len(classes))], ".")
			}
		}
		return tag
	}

	return ""
}

// detectAuth checks if the page has authentication elements
func (e *Extractor) detectAuth(page playwright.Page) bool {
	authIndicators := []string{
		`input[type="password"]`,
		`[name*="password"]`,
		`[name*="login"]`,
		`[name*="email"]`,
		`form[action*="login"]`,
		`form[action*="signin"]`,
		`form[action*="auth"]`,
		`button:has-text("Sign in")`,
		`button:has-text("Log in")`,
		`a:has-text("Sign in")`,
		`a:has-text("Log in")`,
	}

	for _, selector := range authIndicators {
		locator := page.Locator(selector)
		if count, _ := locator.Count(); count > 0 {
			return true
		}
	}

	return false
}

// classifyPage classifies the page type based on its elements
func (e *Extractor) classifyPage(elements *PageElements) string {
	// Check for specific patterns
	if elements.HasAuth {
		return "auth"
	}

	if len(elements.Forms) > 0 {
		for _, form := range elements.Forms {
			if form.FormType == "login" || form.FormType == "signup" {
				return "auth"
			}
			if form.FormType == "search" {
				return "search"
			}
		}
		return "form"
	}

	// Check for list patterns (multiple similar elements)
	// This is simplified - would need more sophisticated detection

	// Check title for clues
	titleLower := strings.ToLower(elements.Title)
	switch {
	case strings.Contains(titleLower, "error") || strings.Contains(titleLower, "404"):
		return "error"
	case strings.Contains(titleLower, "home") || strings.Contains(titleLower, "welcome"):
		return "landing"
	case strings.Contains(titleLower, "dashboard"):
		return "dashboard"
	case strings.Contains(titleLower, "settings") || strings.Contains(titleLower, "profile"):
		return "settings"
	}

	return "generic"
}

// classifyFormType classifies a form based on its fields
func (e *Extractor) classifyFormType(form FormModel) string {
	hasPassword := false
	hasEmail := false
	hasSearch := false
	hasName := false

	for _, field := range form.Fields {
		fieldNameLower := strings.ToLower(field.Name)
		placeholderLower := strings.ToLower(field.Placeholder)

		if field.Type == "password" {
			hasPassword = true
		}
		if field.Type == "email" || strings.Contains(fieldNameLower, "email") {
			hasEmail = true
		}
		if field.Type == "search" || strings.Contains(fieldNameLower, "search") || strings.Contains(placeholderLower, "search") {
			hasSearch = true
		}
		if strings.Contains(fieldNameLower, "name") {
			hasName = true
		}
	}

	// Classify based on field combinations
	switch {
	case hasSearch:
		return "search"
	case hasPassword && hasEmail && !hasName:
		return "login"
	case hasPassword && hasEmail && hasName:
		return "signup"
	case hasEmail && !hasPassword:
		return "contact"
	}

	// Check action URL
	actionLower := strings.ToLower(form.Action)
	switch {
	case strings.Contains(actionLower, "login") || strings.Contains(actionLower, "signin"):
		return "login"
	case strings.Contains(actionLower, "signup") || strings.Contains(actionLower, "register"):
		return "signup"
	case strings.Contains(actionLower, "search"):
		return "search"
	case strings.Contains(actionLower, "checkout"):
		return "checkout"
	case strings.Contains(actionLower, "contact"):
		return "contact"
	}

	return "generic"
}

// detectFieldValidation detects the validation type for a field
func (e *Extractor) detectFieldValidation(field FieldModel) string {
	nameLower := strings.ToLower(field.Name)
	placeholderLower := strings.ToLower(field.Placeholder)

	switch field.Type {
	case "email":
		return "email"
	case "tel":
		return "phone"
	case "url":
		return "url"
	case "number":
		return "number"
	case "password":
		return "password"
	}

	switch {
	case strings.Contains(nameLower, "email"):
		return "email"
	case strings.Contains(nameLower, "phone") || strings.Contains(nameLower, "tel"):
		return "phone"
	case strings.Contains(nameLower, "zip") || strings.Contains(nameLower, "postal"):
		return "postal_code"
	case strings.Contains(placeholderLower, "email"):
		return "email"
	}

	return ""
}

// Helper functions

func extractDomain(url string) string {
	// Simple domain extraction
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	parts := strings.Split(url, "/")
	return parts[0]
}

func isInternalLink(href, baseDomain string) bool {
	if href == "" || href == "#" || strings.HasPrefix(href, "#") {
		return true
	}
	if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
		return true
	}
	if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
		return false
	}
	return strings.Contains(href, baseDomain)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
