package schools

import (
	"sort"
	"strings"
	"unicode"
)

type rankedSchool struct {
	school School
	score  int
}

// Search ranks the canonical school names without maintaining a hand-written
// alias table. The matcher intentionally returns candidates instead of
// guessing a single school for the user.
func Search(items []School, query string, limit int) []School {
	return search(items, query, limit, false)
}

func SearchAll(items []School, query string, limit int) []School {
	return search(items, query, limit, true)
}

func search(items []School, query string, limit int, includeInactive bool) []School {
	if limit < 1 {
		limit = 30
	}
	queryKey := normalizeSearchKey(query)
	ranked := make([]rankedSchool, 0, len(items))
	for _, item := range items {
		if !includeInactive && !item.IsActive {
			continue
		}
		score, ok := matchScore(item.SchoolName, queryKey)
		if ok {
			ranked = append(ranked, rankedSchool{school: item, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].school.SchoolCode < ranked[j].school.SchoolCode
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	result := make([]School, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.school)
	}
	return result
}

func normalizeSearchKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("臺", "台", "台灣", "台湾").Replace(value)
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Han) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func matchScore(name, query string) (int, bool) {
	if query == "" {
		return 1, true
	}
	nameKey := normalizeSearchKey(name)
	searchKey := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(nameKey, "國立"), "市立"), "私立")
	keys := []string{nameKey}
	if searchKey != nameKey {
		keys = append(keys, searchKey)
	}
	best := 0
	matched := false
	for _, key := range keys {
		score, ok := scoreKey(key, query)
		if ok && (!matched || score > best) {
			best = score
			matched = true
		}
	}
	return best, matched
}

func scoreKey(name, query string) (int, bool) {
	if name == query {
		return 1000, true
	}
	if strings.HasPrefix(name, query) {
		return 900 - len([]rune(name)), true
	}
	if strings.Contains(name, query) {
		return 800 - len([]rune(name)), true
	}
	positions, ok := subsequencePositions([]rune(name), []rune(query))
	if !ok {
		return 0, false
	}
	span := positions[len(positions)-1] - positions[0]
	return 600 - span - len([]rune(name)), true
}

func subsequencePositions(name, query []rune) ([]int, bool) {
	if len(query) == 0 {
		return nil, true
	}
	positions := make([]int, 0, len(query))
	queryIndex := 0
	for index, value := range name {
		if value == query[queryIndex] {
			positions = append(positions, index)
			queryIndex++
			if queryIndex == len(query) {
				return positions, true
			}
		}
	}
	return nil, false
}
