package schedule

import (
	"errors"
	"fmt"
)

// ErrReservationRefused marks a legitimate refusal of a reservation mutation
// (not active, already terminal, not found): the caller's answer, not a server
// fault. Store and database failures never carry it.
var ErrReservationRefused = errors.New("maat: reservation request refused")

// refusal keeps the historical message text verbatim (callers and logs match on
// it) while answering errors.Is(ErrReservationRefused).
type refusal struct{ msg string }

func (r refusal) Error() string      { return r.msg }
func (refusal) Is(target error) bool { return target == ErrReservationRefused }

func refuse(format string, a ...any) error { return refusal{fmt.Sprintf(format, a...)} }
