package app

import "testing"

// * [x] **P1_LINUX_008** UDF profile reserves non-payload capacity
// - Description: Calculates usable payload capacity for a nominal image capacity with ten-percent local recovery data.
// - Expected: The result is below nominal capacity and accounts for filesystem, archive, encryption, and parity reservations.
func TestLinuxISOProfile_UsablePayloadCapacityReservesOverhead(t *testing.T) {
	t.Parallel()

	profile := LinuxISOProfile{}
	usable, err := profile.UsablePayloadCapacity(1_000_000_000, 10, 1)
	if err != nil {
		t.Fatalf("UsablePayloadCapacity() error = %v", err)
	}
	if usable <= 0 || usable >= 1_000_000_000 {
		t.Fatalf("usable payload = %d, want value between zero and nominal capacity", usable)
	}

}
