package admissions

import "strings"

// normalizeTimelineEventsForProgram resolves source-document scope notes while
// importing a program's events. The scope selects which program receives an
// event; it is not part of the stored event name.
func normalizeTimelineEventsForProgram(events []TimelineEvent, programName string) []TimelineEvent {
	normalized := make([]TimelineEvent, 0, len(events))
	for _, event := range events {
		keep := true
		for {
			scope, eventName, scoped := splitLeadingTimelineScope(event.Name)
			if !scoped {
				break
			}
			if !timelineScopeAppliesToProgram(scope, programName) {
				keep = false
				break
			}
			event.Name = eventName
		}
		if keep {
			normalized = append(normalized, event)
		}
	}
	return normalized
}

func splitLeadingTimelineScope(name string) (scope, remainder string, ok bool) {
	name = strings.TrimSpace(name)
	opening, closing := "（", "）"
	if strings.HasPrefix(name, "(") {
		opening, closing = "(", ")"
	} else if !strings.HasPrefix(name, opening) {
		return "", name, false
	}

	body := name[len(opening):]
	closingIndex := strings.Index(body, closing)
	if closingIndex < 0 {
		return "", name, false
	}
	scope = strings.TrimSpace(body[:closingIndex])
	if !isProgramTimelineScope(scope) {
		return "", name, false
	}
	remainder = strings.TrimSpace(body[closingIndex+len(closing):])
	return scope, remainder, true
}

func isProgramTimelineScope(scope string) bool {
	if scope == "全系" || scope == "全校" || strings.HasPrefix(scope, "除") {
		return true
	}
	departments := splitProgramTimelineScope(scope)
	if len(departments) == 0 {
		return false
	}
	for _, department := range departments {
		if !strings.Contains(department, "系") && !strings.Contains(department, "學程") &&
			!strings.HasSuffix(department, "組") {
			return false
		}
	}
	return true
}

func timelineScopeAppliesToProgram(scope, programName string) bool {
	if scope == "全系" || scope == "全校" {
		return true
	}
	excluded := strings.HasPrefix(scope, "除")
	if excluded {
		scope = strings.TrimSpace(strings.TrimPrefix(scope, "除"))
	}
	programText := compactProgramScopeText(programName)
	matched := false
	for _, department := range splitProgramTimelineScope(scope) {
		departmentText := compactProgramScopeText(department)
		if departmentText != "" && strings.Contains(programText, departmentText) {
			matched = true
			break
		}
	}
	if excluded {
		return !matched
	}
	return matched
}

func splitProgramTimelineScope(scope string) []string {
	parts := strings.FieldsFunc(scope, func(r rune) bool {
		return r == '、' || r == '，' || r == ','
	})
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func compactProgramScopeText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), "")
}
