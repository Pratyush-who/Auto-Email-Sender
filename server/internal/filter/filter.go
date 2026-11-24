package filter

import (
	"fmt"
	"strings"

	"emailauto-server/internal/model"
)

// ApplyFilters is a pure function that evaluates a Post against Settings.
// Returns (pass, reason). When pass is true, reason is empty.
func ApplyFilters(post model.Post, settings model.Settings) (bool, string) {
	// 1. Skip unpaid posts
	if settings.SkipUnpaid {
		if strings.EqualFold(strings.TrimSpace(post.PaidStatus), "unpaid") {
			return false, "post is unpaid"
		}
	}

	// 2. Skip unknown paid status
	if settings.SkipUnknownPaid {
		cleaned := strings.TrimSpace(post.PaidStatus)
		if cleaned == "" || strings.EqualFold(cleaned, "unknown") {
			return false, "post paid status is unknown"
		}
	}

	// 3. Minimum stipend check (if post has stipend data)
	if settings.MinStipend != nil && post.Extracted.Stipend != nil {
		if *post.Extracted.Stipend < *settings.MinStipend {
			return false, fmt.Sprintf("stipend %.0f below minimum %.0f", *post.Extracted.Stipend, *settings.MinStipend)
		}
	}

	// 4. Work mode check:
	// Rule: for hybrid treat as remote; for not mentioned/unknown assume remote.
	// Onsite posts are evaluated against allowed work modes.
	if len(settings.WorkModes) > 0 {
		effectiveWorkMode := strings.ToLower(strings.TrimSpace(post.WorkMode))
		if effectiveWorkMode == "hybrid" || effectiveWorkMode == "" || effectiveWorkMode == "unknown" {
			effectiveWorkMode = "remote"
		}

		matched := false
		for _, wm := range settings.WorkModes {
			if strings.EqualFold(strings.TrimSpace(wm), effectiveWorkMode) {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("work mode '%s' not in allowed work modes", post.WorkMode)
		}
	}

	// 5. Allowed locations check (if post specifies a location)
	if len(settings.AllowedLocations) > 0 && strings.TrimSpace(post.Location) != "" {
		matched := false
		postLocLower := strings.ToLower(post.Location)
		for _, loc := range settings.AllowedLocations {
			cleaned := strings.ToLower(strings.TrimSpace(loc))
			if cleaned != "" && strings.Contains(postLocLower, cleaned) {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("location '%s' not in allowed locations", post.Location)
		}
	}

	// 6. Experience bounds check (if post has experience data)
	if post.Extracted.ExperienceYears != nil {
		if settings.MinExperienceYears != nil && *post.Extracted.ExperienceYears < *settings.MinExperienceYears {
			return false, fmt.Sprintf("experience %.1f below minimum %.1f", *post.Extracted.ExperienceYears, *settings.MinExperienceYears)
		}
		if settings.MaxExperienceYears != nil && *post.Extracted.ExperienceYears > *settings.MaxExperienceYears {
			return false, fmt.Sprintf("experience %.1f exceeds maximum %.1f", *post.Extracted.ExperienceYears, *settings.MaxExperienceYears)
		}
	}

	// 7. Exclude keywords check
	if len(settings.ExcludeKeywords) > 0 {
		corpus := strings.ToLower(fmt.Sprintf("%s %s %s %s", post.Role, post.Text, post.Company, post.Location))
		for _, kw := range settings.ExcludeKeywords {
			cleaned := strings.ToLower(strings.TrimSpace(kw))
			if cleaned != "" && strings.Contains(corpus, cleaned) {
				return false, fmt.Sprintf("contains excluded keyword: %s", kw)
			}
		}
	}

	// 8. Include keywords check
	if len(settings.IncludeKeywords) > 0 {
		corpus := strings.ToLower(fmt.Sprintf("%s %s %s %s", post.Role, post.Text, post.Company, post.Location))
		matched := false
		for _, kw := range settings.IncludeKeywords {
			cleaned := strings.ToLower(strings.TrimSpace(kw))
			if cleaned != "" && strings.Contains(corpus, cleaned) {
				matched = true
				break
			}
		}
		if !matched {
			return false, "post missing required include keywords"
		}
	}

	return true, ""
}
