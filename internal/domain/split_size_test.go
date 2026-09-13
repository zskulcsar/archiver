package domain

import "testing"

// * [x] **P1_CORE_018** Minimum split-size parsing and resolution
// - Description: Parses zero, decimal-unit, and percentage thresholds against usable disc capacity.
// - Expected: Valid thresholds resolve exactly and malformed, excessive, or impractical values fail.
func TestParseMinimumSplitSize(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr bool
	}{
		"zero megabytes":       {input: "0MB", want: 0},
		"absolute megabytes":   {input: "500MB", want: 500_000_000},
		"absolute gigabytes":   {input: "1GB", want: 1_000_000_000},
		"percentage":           {input: "1%", want: 20_000_000},
		"maximum percentage":   {input: "100%", want: 2_000_000_000},
		"lowercase":            {input: "1mb", wantErr: true},
		"fraction":             {input: "1.5%", wantErr: true},
		"bare value":           {input: "1", wantErr: true},
		"excessive percentage": {input: "101%", wantErr: true},
		"larger than disc":     {input: "3GB", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinimumSplitSize(tc.input, 2_000_000_000)
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParseMinimumSplitSize() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMinimumSplitSize() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseMinimumSplitSize() = %d, want %d", got, tc.want)
			}
		})
	}
}

// * [x] **P1_CORE_019** Zero split threshold fills remaining capacity
// - Description: Plans a file across a sub-gigabyte remainder when the configured threshold is zero.
// - Expected: The first disc is full and no capacity is left unused by the split policy.
func TestPlanWithMinimumSplitSize_ZeroFillsRemainder(t *testing.T) {
	t.Parallel()

	plan, err := PlanWithMinimumSplitSize([]Source{
		{LogicalPath: "a", Size: 1_500_000_000},
		{LogicalPath: "b", Size: 1_200_000_000},
	}, 2_000_000_000, 0)
	if err != nil {
		t.Fatalf("PlanWithMinimumSplitSize() error = %v", err)
	}
	if got, want := plan.Discs[0].Used, int64(2_000_000_000); got != want {
		t.Fatalf("first disc used = %d, want %d", got, want)
	}
	if got := plan.Discs[0].Unused; got != 0 {
		t.Fatalf("first disc unused = %d, want 0", got)
	}
}
