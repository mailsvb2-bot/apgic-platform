package demand

import (
	"strings"
	"unicode"
)

const (
	ReasonInterpretationLexicon = "INTERPRETATION_LEXICON_V1"
	ReasonNeedsCorrection       = "INTERPRETATION_NEEDS_CORRECTION"
	ReasonNotADiagnosis         = "INTERPRETATION_NOT_A_DIAGNOSIS"
)

// InterpretationNotice is shown to the person before they confirm.
// The lexicon never emits a diagnosis claim.
const InterpretationNotice = "Это предположение по вашим словам, не диагноз и не медицинское заключение. Проверьте темы и цели и исправьте их, если мы поняли неверно."

type suggestion struct {
	Topics      []string
	Goals       []string
	ReasonCodes []string
}

type lexiconEntry struct {
	topic   string
	goal    string
	needles []string
}

var lexicon = []lexiconEntry{
	{topic: "anxiety", goal: "cope-with-distress", needles: []string{"тревог", "волнен", "паник", "выступлен", "anxiety"}},
	{topic: "sleep", goal: "restore-sleep", needles: []string{"бессон", "засып", "сон", "сплю", "sleep"}},
	{topic: "career", goal: "career-decision", needles: []string{"карьер", "работ", "выгоран", "увольн", "career"}},
	{topic: "relationships", goal: "improve-communication", needles: []string{"отношен", "семь", "партн", "конфликт", "relationship"}},
}

func interpret(freeText string) suggestion {
	folded := fold(freeText)
	topics := make([]string, 0, 2)
	goals := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, entry := range lexicon {
		if !containsAny(folded, entry.needles) {
			continue
		}
		if _, ok := seen[entry.topic]; ok {
			continue
		}
		seen[entry.topic] = struct{}{}
		topics = append(topics, entry.topic)
		goals = append(goals, entry.goal)
	}
	reasons := []string{ReasonInterpretationLexicon, ReasonNotADiagnosis}
	if len(topics) == 0 {
		reasons = append(reasons, ReasonNeedsCorrection)
	}
	return suggestion{Topics: topics, Goals: goals, ReasonCodes: reasons}
}

func fold(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func containsAny(folded string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(folded, needle) {
			return true
		}
	}
	return false
}

func cleanToken(value string) string {
	return strings.TrimSpace(value)
}

func isBlank(value string) bool {
	for _, r := range value {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
