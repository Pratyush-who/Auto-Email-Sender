package filter

import (
	"testing"

	"emailauto-server/internal/model"
)

func floatPtr(val float64) *float64 {
	return &val
}

func TestApplyFilters(t *testing.T) {
	tests := []struct {
		name         string
		post         model.Post
		settings     model.Settings
		expectedPass bool
		expectedMsg  string
	}{
		{
			name: "Passes with default settings and standard paid post",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "remote",
				Location:   "Bangalore",
				Role:       "Backend Engineer",
				Text:       "We are hiring a Go developer",
			},
			settings:     model.DefaultSettings(),
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when post is unpaid and SkipUnpaid is true",
			post: model.Post{
				PaidStatus: "unpaid",
			},
			settings: model.Settings{
				SkipUnpaid: true,
			},
			expectedPass: false,
			expectedMsg:  "post is unpaid",
		},
		{
			name: "Passes when post is unpaid but SkipUnpaid is false",
			post: model.Post{
				PaidStatus: "unpaid",
			},
			settings: model.Settings{
				SkipUnpaid: false,
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when paid status is unknown and SkipUnknownPaid is true",
			post: model.Post{
				PaidStatus: "unknown",
			},
			settings: model.Settings{
				SkipUnknownPaid: true,
			},
			expectedPass: false,
			expectedMsg:  "post paid status is unknown",
		},
		{
			name: "Fails when paid status is empty string and SkipUnknownPaid is true",
			post: model.Post{
				PaidStatus: "",
			},
			settings: model.Settings{
				SkipUnknownPaid: true,
			},
			expectedPass: false,
			expectedMsg:  "post paid status is unknown",
		},
		{
			name: "Passes when paid status is unknown but SkipUnknownPaid is false",
			post: model.Post{
				PaidStatus: "unknown",
			},
			settings: model.Settings{
				SkipUnknownPaid: false,
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when stipend is lower than MinStipend",
			post: model.Post{
				PaidStatus: "paid",
				Extracted: model.PostExtracted{
					Stipend: floatPtr(12000),
				},
			},
			settings: model.Settings{
				MinStipend: floatPtr(20000),
			},
			expectedPass: false,
			expectedMsg:  "stipend 12000 below minimum 20000",
		},
		{
			name: "Passes when stipend is equal to or greater than MinStipend",
			post: model.Post{
				PaidStatus: "paid",
				Extracted: model.PostExtracted{
					Stipend: floatPtr(25000),
				},
			},
			settings: model.Settings{
				MinStipend: floatPtr(20000),
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when work mode is not in allowed list",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "onsite",
			},
			settings: model.Settings{
				WorkModes: []string{"remote", "hybrid"},
			},
			expectedPass: false,
			expectedMsg:  "work mode 'onsite' not in allowed work modes",
		},
		{
			name: "Passes when work mode matches case-insensitively",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "Remote",
			},
			settings: model.Settings{
				WorkModes: []string{"remote", "hybrid"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Hybrid post treated as remote and passes when only remote is allowed",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "hybrid",
			},
			settings: model.Settings{
				WorkModes: []string{"remote"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Unmentioned work mode assumed remote and passes when only remote is allowed",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "",
			},
			settings: model.Settings{
				WorkModes: []string{"remote"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Unknown work mode assumed remote and passes when only remote is allowed",
			post: model.Post{
				PaidStatus: "paid",
				WorkMode:   "unknown",
			},
			settings: model.Settings{
				WorkModes: []string{"remote"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when location is not in allowed locations",
			post: model.Post{
				PaidStatus: "paid",
				Location:   "London, UK",
			},
			settings: model.Settings{
				AllowedLocations: []string{"Bangalore", "San Francisco"},
			},
			expectedPass: false,
			expectedMsg:  "location 'London, UK' not in allowed locations",
		},
		{
			name: "Passes when location is substring of allowed location",
			post: model.Post{
				PaidStatus: "paid",
				Location:   "Bangalore, India",
			},
			settings: model.Settings{
				AllowedLocations: []string{"Bangalore", "San Francisco"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when experience is below MinExperienceYears",
			post: model.Post{
				PaidStatus: "paid",
				Extracted: model.PostExtracted{
					ExperienceYears: floatPtr(1.0),
				},
			},
			settings: model.Settings{
				MinExperienceYears: floatPtr(2.0),
			},
			expectedPass: false,
			expectedMsg:  "experience 1.0 below minimum 2.0",
		},
		{
			name: "Fails when experience exceeds MaxExperienceYears",
			post: model.Post{
				PaidStatus: "paid",
				Extracted: model.PostExtracted{
					ExperienceYears: floatPtr(6.0),
				},
			},
			settings: model.Settings{
				MaxExperienceYears: floatPtr(5.0),
			},
			expectedPass: false,
			expectedMsg:  "experience 6.0 exceeds maximum 5.0",
		},
		{
			name: "Passes when experience is within min and max bounds",
			post: model.Post{
				PaidStatus: "paid",
				Extracted: model.PostExtracted{
					ExperienceYears: floatPtr(3.0),
				},
			},
			settings: model.Settings{
				MinExperienceYears: floatPtr(2.0),
				MaxExperienceYears: floatPtr(4.0),
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Fails when post contains an excluded keyword",
			post: model.Post{
				PaidStatus: "paid",
				Role:       "Blockchain Developer",
				Text:       "Looking for smart contract and crypto specialists",
			},
			settings: model.Settings{
				ExcludeKeywords: []string{"crypto", "blockchain"},
			},
			expectedPass: false,
			expectedMsg:  "contains excluded keyword: crypto",
		},
		{
			name: "Fails when post misses required include keywords",
			post: model.Post{
				PaidStatus: "paid",
				Role:       "DevOps Engineer",
				Text:       "Working with Kubernetes and AWS",
			},
			settings: model.Settings{
				IncludeKeywords: []string{"golang", "react"},
			},
			expectedPass: false,
			expectedMsg:  "post missing required include keywords",
		},
		{
			name: "Passes when post contains at least one include keyword",
			post: model.Post{
				PaidStatus: "paid",
				Role:       "Go Backend Engineer",
				Text:       "Building microservices in Golang",
			},
			settings: model.Settings{
				IncludeKeywords: []string{"golang", "react"},
			},
			expectedPass: true,
			expectedMsg:  "",
		},
		{
			name: "Passes everything when all optional filters are disabled (nil/empty)",
			post: model.Post{
				PaidStatus: "paid",
				Location:   "Anywhere",
				WorkMode:   "hybrid",
				Role:       "Designer",
				Text:       "UI/UX role",
			},
			settings: model.Settings{
				MinStipend:         nil,
				AllowedLocations:   nil,
				WorkModes:          nil,
				MinExperienceYears: nil,
				MaxExperienceYears: nil,
				IncludeKeywords:    nil,
				ExcludeKeywords:    nil,
				SkipUnpaid:         false,
				SkipUnknownPaid:    false,
			},
			expectedPass: true,
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pass, reason := ApplyFilters(tt.post, tt.settings)
			if pass != tt.expectedPass {
				t.Errorf("ApplyFilters() pass = %v, want %v (reason: %q)", pass, tt.expectedPass, reason)
			}
			if !tt.expectedPass && reason != tt.expectedMsg {
				t.Errorf("ApplyFilters() reason = %q, want %q", reason, tt.expectedMsg)
			}
		})
	}
}
