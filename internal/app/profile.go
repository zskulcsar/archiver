package app

import "fmt"

const (
	udfFilesystemReservation = 64_000_000
	metadataReservation      = 4_000_000
	encryptionReservation    = 1_000_000
	paxMemberReservation     = 8_192
	paxPaddingPercent        = 1
)

// LinuxISOProfile defines the initial xorriso ISO9660 Level 3 image capacity model.
type LinuxISOProfile struct{}

// UsablePayloadCapacity returns a conservatively reserved payload capacity for one disc.
func (LinuxISOProfile) UsablePayloadCapacity(nominalCapacity int64, lossPercent, sourceCount int) (int64, error) {
	if nominalCapacity <= 0 {
		return 0, fmt.Errorf("nominal capacity must be positive")
	}
	if lossPercent < 0 || lossPercent > 50 {
		return 0, fmt.Errorf("loss tolerance must be between zero and 50 percent")
	}
	if sourceCount < 0 || int64(sourceCount) > (int64(^uint64(0)>>1)-udfFilesystemReservation-metadataReservation-encryptionReservation)/paxMemberReservation {
		return 0, fmt.Errorf("source count exceeds supported capacity model range")
	}
	fixedReservation := int64(udfFilesystemReservation+metadataReservation+encryptionReservation) + int64(sourceCount)*paxMemberReservation
	if nominalCapacity <= fixedReservation {
		return 0, fmt.Errorf("nominal capacity %d is too small for the Linux UDF image profile", nominalCapacity)
	}
	available := nominalCapacity - fixedReservation
	denominator := int64((100 + paxPaddingPercent) * (100 + lossPercent))
	usable := available * 10_000 / denominator
	if usable <= 0 {
		return 0, fmt.Errorf("nominal capacity %d leaves no payload capacity", nominalCapacity)
	}
	return usable, nil
}
