package rules

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/singtail/singtail/internal/contract"
)

var ErrNoValidImportedRules = errors.New("no valid rules found in import text")

type importRuleSpec struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Action  string `json:"action"`
	Enabled *bool  `json:"enabled"`
}

// ImportText parses rules from JSON arrays, Clash YAML list lines, or comma-separated lines
// (standalone or mixed), inserts all valid rules at the top of the user rule zone, and returns
// the number of imported rules. Returns an error if zero valid rules are parsed.
func (m *Manager) ImportText(rawText string) (int, error) {
	specs, err := parseImportText(rawText)
	if err != nil {
		return 0, err
	}
	if len(specs) == 0 {
		return 0, ErrNoValidImportedRules
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	imported := make([]contract.Rule, 0, len(specs))
	for _, spec := range specs {
		enabled := true
		if spec.Enabled != nil {
			enabled = *spec.Enabled
		}
		imported = append(imported, contract.Rule{
			ID:      m.nextIDLocked("rule"),
			Type:    contract.RuleType(spec.Type),
			Payload: spec.Payload,
			Action:  contract.RuleAction(spec.Action),
			Enabled: enabled,
			Locked:  false,
		})
	}

	m.insertCustomRulesTopLocked(imported)
	return len(imported), nil
}

func parseImportText(rawText string) ([]importRuleSpec, error) {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return nil, ErrNoValidImportedRules
	}

	// Fast path when the entire payload is a standalone JSON array.
	if strings.HasPrefix(trimmed, "[") {
		var items []importRuleSpec
		if err := json.Unmarshal([]byte(trimmed), &items); err == nil {
			valid := filterValidSpecs(items)
			if len(valid) == 0 {
				return nil, ErrNoValidImportedRules
			}
			return valid, nil
		}
	}

	normalized := strings.ReplaceAll(rawText, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	var valid []importRuleSpec

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if isIgnoredLine(line) {
			continue
		}

		if strings.HasPrefix(line, "[") {
			items, nextIdx, ok := tryParseJSONArrayBlock(lines, i)
			if ok {
				valid = append(valid, filterValidSpecs(items)...)
				i = nextIdx
				continue
			}
		}

		if strings.HasPrefix(line, "{") {
			item, nextIdx, ok := tryParseJSONObjectBlock(lines, i)
			if ok {
				valid = append(valid, filterValidSpecs([]importRuleSpec{item})...)
				i = nextIdx
				continue
			}
		}

		if spec, ok := parseRuleLine(line); ok {
			valid = append(valid, spec)
		}
	}

	if len(valid) == 0 {
		return nil, ErrNoValidImportedRules
	}
	return valid, nil
}

func isIgnoredLine(line string) bool {
	if line == "" {
		return true
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
		return true
	}
	if strings.EqualFold(line, "rules:") {
		return true
	}
	return false
}

func tryParseJSONArrayBlock(lines []string, start int) ([]importRuleSpec, int, bool) {
	var buf strings.Builder
	for j := start; j < len(lines); j++ {
		if j > start {
			buf.WriteByte('\n')
		}
		buf.WriteString(lines[j])
		trimmed := strings.TrimSpace(lines[j])
		if strings.HasSuffix(trimmed, "]") {
			var items []importRuleSpec
			if err := json.Unmarshal([]byte(buf.String()), &items); err == nil {
				return items, j, true
			}
		}
	}
	return nil, start, false
}

func tryParseJSONObjectBlock(lines []string, start int) (importRuleSpec, int, bool) {
	var buf strings.Builder
	for j := start; j < len(lines); j++ {
		if j > start {
			buf.WriteByte('\n')
		}
		buf.WriteString(lines[j])
		trimmed := strings.TrimSpace(lines[j])
		if strings.HasSuffix(trimmed, "}") {
			var item importRuleSpec
			if err := json.Unmarshal([]byte(buf.String()), &item); err == nil {
				return item, j, true
			}
		}
	}
	return importRuleSpec{}, start, false
}

func parseRuleLine(line string) (importRuleSpec, bool) {
	cleaned := strings.TrimSpace(line)
	if strings.HasPrefix(cleaned, "-") {
		cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "-"))
	}
	cleaned = stripInlineComment(cleaned)
	cleaned = stripMatchingQuotes(cleaned)

	parts := strings.Split(cleaned, ",")
	if len(parts) < 3 {
		return importRuleSpec{}, false
	}
	if len(parts) > 3 {
		// Allow Clash optional trailing flags such as "no-resolve".
		if len(parts) != 4 || !strings.EqualFold(strings.TrimSpace(parts[3]), "no-resolve") {
			return importRuleSpec{}, false
		}
	}

	return validateImportSpec(importRuleSpec{
		Type:    parts[0],
		Payload: parts[1],
		Action:  parts[2],
	})
}

func stripMatchingQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}

func stripInlineComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if i > 0 && (s[i-1] == ' ' || s[i-1] == '\t' || s[i-1] == '\'' || s[i-1] == '"') {
			if ch == '#' || (ch == '/' && i+1 < len(s) && s[i+1] == '/') {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func filterValidSpecs(items []importRuleSpec) []importRuleSpec {
	out := make([]importRuleSpec, 0, len(items))
	for _, item := range items {
		if valid, ok := validateImportSpec(item); ok {
			out = append(out, valid)
		}
	}
	return out
}

func validateImportSpec(item importRuleSpec) (importRuleSpec, bool) {
	ruleType := contract.RuleType(strings.ToUpper(strings.TrimSpace(item.Type)))
	if !isValidUserRuleType(ruleType) {
		return importRuleSpec{}, false
	}

	payload := strings.TrimSpace(item.Payload)
	if payload == "" {
		return importRuleSpec{}, false
	}

	action, ok := normalizeImportAction(item.Action)
	if !ok {
		return importRuleSpec{}, false
	}

	return importRuleSpec{
		Type:    string(ruleType),
		Payload: payload,
		Action:  string(action),
		Enabled: item.Enabled,
	}, true
}
