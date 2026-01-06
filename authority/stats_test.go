package authority

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseRelativeDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func() time.Time
		wantErr  bool
	}{
		{
			name:  "empty string returns today",
			input: "",
			expected: func() time.Time {
				return time.Now()
			},
			wantErr: false,
		},
		{
			name:  "today returns today",
			input: "today",
			expected: func() time.Time {
				return time.Now()
			},
			wantErr: false,
		},
		{
			name:  "365 days ago",
			input: "365 days ago",
			expected: func() time.Time {
				return time.Now().AddDate(0, 0, -365)
			},
			wantErr: false,
		},
		{
			name:  "30 days ago",
			input: "30 days ago",
			expected: func() time.Time {
				return time.Now().AddDate(0, 0, -30)
			},
			wantErr: false,
		},
		{
			name:  "1 day ago",
			input: "1 day ago",
			expected: func() time.Time {
				return time.Now().AddDate(0, 0, -1)
			},
			wantErr: false,
		},
		{
			name:  "standard date format",
			input: "2024-01-15",
			expected: func() time.Time {
				t, _ := time.Parse("2006-01-02", "2024-01-15")
				return t
			},
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not a date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRelativeDate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Allow 5 second tolerance for "now" comparisons.
				expected := tt.expected()
				diff := result.Sub(expected)
				if diff < 0 {
					diff = -diff
				}
				assert.True(t, diff < 5*time.Second, "Time difference too large: %v", diff)
			}
		})
	}
}

func TestPostcodeStatsStruct(t *testing.T) {
	// Test that PostcodeStats struct has correct default values.
	stats := PostcodeStats{}
	assert.Equal(t, 0, stats.Offer)
	assert.Equal(t, 0, stats.Wanted)
	assert.Equal(t, 0, stats.Searches)
	assert.Equal(t, float64(0), stats.Weight)
	assert.Equal(t, 0, stats.Replies)
	assert.Equal(t, 0, stats.Outcomes)
}

func TestConstants(t *testing.T) {
	// Verify constants match PHP values.
	assert.Equal(t, "Offer", TypeOffer)
	assert.Equal(t, "Wanted", TypeWanted)
	assert.Equal(t, "Searches", StatSearches)
	assert.Equal(t, "Weight", StatWeight)
	assert.Equal(t, "Outcomes", StatOutcomes)
	assert.Equal(t, "Replies", StatReplies)
}
