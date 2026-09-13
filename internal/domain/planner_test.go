package domain

import "testing"

// * [x] **P1_CORE_002** Deterministic canonical allocation
// - Description: Plans unordered source entries using canonical logical-path byte ordering rather than caller order.
// - Expected: Disc assignments are stable and ordered by logical archive path.
func TestPlan_SortsSourcesByLogicalPath(t *testing.T) {
	t.Parallel()

	plan, err := Plan([]Source{
		{LogicalPath: "zeta.txt", Size: 4},
		{LogicalPath: "alpha.txt", Size: 4},
	}, 10)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if got, want := len(plan.Discs), 1; got != want {
		t.Fatalf("len(Plan().Discs) = %d, want %d", got, want)
	}
	parts := plan.Discs[0].Parts
	if got, want := parts[0].LogicalPath, "alpha.txt"; got != want {
		t.Fatalf("first logical path = %q, want %q", got, want)
	}
	if got, want := parts[1].LogicalPath, "zeta.txt"; got != want {
		t.Fatalf("second logical path = %q, want %q", got, want)
	}
}

// * [x] **P1_CORE_020** Symlink archive-member planning
// - Description: Plans a symlink as a zero-byte archive member alongside regular file parts.
// - Expected: The planned member retains only its logical path and archive-relative target.
func TestPlan_PreservesSymlinkMember(t *testing.T) {
	t.Parallel()

	plan, err := Plan([]Source{
		{LogicalPath: "photo.jpg", Size: 4},
		{LogicalPath: "latest.jpg", SymlinkTarget: "photo.jpg"},
	}, 10)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if got, want := plan.Discs[0].Parts[0], (Part{LogicalPath: "latest.jpg", SymlinkTarget: "photo.jpg"}); got != want {
		t.Fatalf("first planned part = %#v, want %#v", got, want)
	}
	if got, want := plan.Discs[0].Parts[1], (Part{LogicalPath: "photo.jpg", Size: 4}); got != want {
		t.Fatalf("second planned part = %#v, want %#v", got, want)
	}
}

// * [x] **P1_CORE_003** Minimum split-part threshold
// - Description: Leaves a sub-gigabyte remainder unused rather than splitting a file at that boundary.
// - Expected: The complete file starts on the following disc and the unused capacity is recorded.
func TestPlan_DoesNotSplitAtSubGigabyteBoundary(t *testing.T) {
	t.Parallel()

	plan, err := PlanWithMinimumSplitSize([]Source{
		{LogicalPath: "a", Size: 1_500_000_000},
		{LogicalPath: "b", Size: 1_200_000_000},
	}, 2_000_000_000, 1_000_000_000)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if got, want := len(plan.Discs), 2; got != want {
		t.Fatalf("len(Plan().Discs) = %d, want %d", got, want)
	}
	if got, want := plan.Discs[0].Unused, int64(500_000_000); got != want {
		t.Fatalf("first disc unused = %d, want %d", got, want)
	}
	if got, want := plan.Discs[1].Parts[0].LogicalPath, "b"; got != want {
		t.Fatalf("second disc first path = %q, want %q", got, want)
	}
}

// * [x] **P1_CORE_004** Boundary split with a one-gigabyte part
// - Description: Splits a file when the current disc can hold at least one gigabyte of its remaining bytes.
// - Expected: Ordered parts cover the original source without gaps or overlap.
func TestPlan_SplitsAtGigabyteBoundary(t *testing.T) {
	t.Parallel()

	plan, err := Plan([]Source{
		{LogicalPath: "a", Size: 1_000_000_000},
		{LogicalPath: "b", Size: 2_000_000_000},
	}, 2_000_000_000)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	first := plan.Discs[0].Parts[1]
	second := plan.Discs[1].Parts[0]
	if first.Offset != 0 || first.Size != 1_000_000_000 {
		t.Fatalf("first part = %+v, want offset 0 and size 1000000000", first)
	}
	if second.Offset != 1_000_000_000 || second.Size != 1_000_000_000 {
		t.Fatalf("second part = %+v, want offset 1000000000 and size 1000000000", second)
	}
}
