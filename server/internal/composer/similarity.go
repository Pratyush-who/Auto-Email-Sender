package composer

import (
	"regexp"
	"strings"
)

var wordPunctRegex = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// TokenizeWords splits text into lowercase alphanumeric tokens.
func TokenizeWords(text string) []string {
	cleaned := wordPunctRegex.ReplaceAllString(strings.ToLower(text), " ")
	fields := strings.Fields(cleaned)
	return fields
}

// BuildNGrams creates a set of contiguous n-word shingles.
func BuildNGrams(words []string, n int) map[string]struct{} {
	set := make(map[string]struct{})
	if len(words) < n {
		if len(words) > 0 {
			set[strings.Join(words, " ")] = struct{}{}
		}
		return set
	}

	for i := 0; i <= len(words)-n; i++ {
		ngram := strings.Join(words[i:i+n], " ")
		set[ngram] = struct{}{}
	}
	return set
}

// CalculateSimilarity returns the Jaccard similarity score between 0.0 and 1.0 using bi-grams.
func CalculateSimilarity(textA, textB string) float64 {
	wordsA := TokenizeWords(textA)
	wordsB := TokenizeWords(textB)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	ngramsA := BuildNGrams(wordsA, 2)
	ngramsB := BuildNGrams(wordsB, 2)

	intersection := 0
	for k := range ngramsA {
		if _, ok := ngramsB[k]; ok {
			intersection++
		}
	}

	union := len(ngramsA) + len(ngramsB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// IsNearIdentical checks if candidate email body has similarity exceeding threshold against any existing bodies.
func IsNearIdentical(candidate string, existingBodies []string, threshold float64) (bool, float64) {
	if threshold <= 0 {
		threshold = 0.70
	}

	maxSim := 0.0
	for _, existing := range existingBodies {
		sim := CalculateSimilarity(candidate, existing)
		if sim > maxSim {
			maxSim = sim
		}
		if sim >= threshold {
			return true, sim
		}
	}

	return false, maxSim
}
