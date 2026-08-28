package browser

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	defaultFindLimit = 10
	maxFindLimit     = 50
	maxQueryRunes    = 200
	defaultWait      = 10 * time.Second
	maxWait          = 30 * time.Second
	matchContext     = 100
)

func NormalizeFindRequest(query string, limit int) (string, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", 0, errors.New("find query must not be empty")
	}
	if len([]rune(query)) > maxQueryRunes {
		return "", 0, fmt.Errorf("find query must not exceed %d characters", maxQueryRunes)
	}
	if limit == 0 {
		limit = defaultFindLimit
	}
	if limit < 1 || limit > maxFindLimit {
		return "", 0, fmt.Errorf("find limit must be between 1 and %d", maxFindLimit)
	}
	return query, limit, nil
}

func FindSnapshot(snapshot Snapshot, query string, limit int) (FindResult, error) {
	query, limit, err := NormalizeFindRequest(query, limit)
	if err != nil {
		return FindResult{}, err
	}
	result := FindResult{
		Query:      query,
		URL:        snapshot.URL,
		Title:      snapshot.Title,
		Backend:    snapshot.Backend,
		Generation: snapshot.Generation,
		Matches:    []FindMatch{},
	}
	lowerQuery := strings.ToLower(query)
	seen := make(map[string]struct{})
	add := func(match FindMatch) bool {
		key := match.Ref + "\x00" + match.Href + "\x00" + match.Text
		if _, exists := seen[key]; exists {
			return true
		}
		seen[key] = struct{}{}
		if len(result.Matches) >= limit {
			result.Truncated = true
			return false
		}
		result.Matches = append(result.Matches, match)
		return true
	}

	for _, element := range snapshot.Elements {
		label := strings.TrimSpace(strings.Join([]string{element.Role, element.Name, element.Value}, " "))
		if strings.Contains(strings.ToLower(label), lowerQuery) && !add(FindMatch{Text: label, Ref: element.Ref}) {
			return result, nil
		}
	}
	for _, excerpt := range textExcerpts(snapshot.Text, query, limit+1) {
		if !add(FindMatch{Text: excerpt}) {
			break
		}
	}
	return result, nil
}

func NormalizeWaitCondition(condition WaitCondition) (WaitCondition, error) {
	condition.Text = strings.TrimSpace(condition.Text)
	condition.URLContains = strings.TrimSpace(condition.URLContains)
	if (condition.Text == "") == (condition.URLContains == "") {
		return WaitCondition{}, errors.New("provide exactly one wait condition: text or url_contains")
	}
	if len([]rune(condition.Text)) > maxQueryRunes || len([]rune(condition.URLContains)) > maxQueryRunes {
		return WaitCondition{}, fmt.Errorf("wait value must not exceed %d characters", maxQueryRunes)
	}
	if condition.Timeout == 0 {
		condition.Timeout = defaultWait
	}
	if condition.Timeout < 250*time.Millisecond || condition.Timeout > maxWait {
		return WaitCondition{}, fmt.Errorf("wait timeout must be between 250ms and %s", maxWait)
	}
	return condition, nil
}

func WaitSatisfied(snapshot Snapshot, condition WaitCondition) bool {
	if condition.Text != "" {
		return strings.Contains(strings.ToLower(snapshot.Text), strings.ToLower(condition.Text))
	}
	return strings.Contains(strings.ToLower(snapshot.URL), strings.ToLower(condition.URLContains))
}

func textExcerpts(text, query string, limit int) []string {
	textRunes := []rune(text)
	lowerText := lowerRunes(textRunes)
	lowerQuery := lowerRunes([]rune(query))
	if len(lowerQuery) == 0 || len(lowerText) < len(lowerQuery) {
		return nil
	}
	excerpts := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for start := 0; start+len(lowerQuery) <= len(lowerText); {
		index := indexRunes(lowerText[start:], lowerQuery)
		if index < 0 {
			break
		}
		matchStart := start + index
		excerptStart := max(0, matchStart-matchContext)
		excerptEnd := min(len(textRunes), matchStart+len(lowerQuery)+matchContext)
		excerpt := strings.Join(strings.Fields(string(textRunes[excerptStart:excerptEnd])), " ")
		if excerptStart > 0 {
			excerpt = "…" + excerpt
		}
		if excerptEnd < len(textRunes) {
			excerpt += "…"
		}
		if _, exists := seen[excerpt]; !exists {
			seen[excerpt] = struct{}{}
			excerpts = append(excerpts, excerpt)
			if len(excerpts) >= limit {
				break
			}
		}
		start = matchStart + max(1, len(lowerQuery))
	}
	return excerpts
}

func lowerRunes(value []rune) []rune {
	result := make([]rune, len(value))
	for index, character := range value {
		result[index] = unicode.ToLower(character)
	}
	return result
}

func indexRunes(value, query []rune) int {
	for index := 0; index+len(query) <= len(value); index++ {
		matched := true
		for offset := range query {
			if value[index+offset] != query[offset] {
				matched = false
				break
			}
		}
		if matched {
			return index
		}
	}
	return -1
}
