package routerstore

import "github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"

func (rs *RemoteStore) CompareAndSwapState(key, expected string, exists bool, value string) (bool, error) {
	var swapped bool
	err := rs.call("CompareAndSwapState", []any{key, expected, exists, value}, &swapped)
	return swapped, err
}

func (rs *RemoteStore) MaatReserve(req schedule.Reservation, queue bool) (schedule.ReserveResult, error) {
	var out schedule.ReserveResult
	err := rs.call("MaatReserve", []any{req, queue}, &out)
	return out, err
}

func (rs *RemoteStore) MaatMutate(action, id, value string) (*schedule.Reservation, error) {
	var out *schedule.Reservation
	err := rs.call("MaatMutate", []any{action, id, value}, &out)
	return out, err
}

func (rs *RemoteStore) MaatRead(kind, resource string) ([]schedule.Reservation, error) {
	var out []schedule.Reservation
	err := rs.call("MaatRead", []any{kind, resource}, &out)
	return out, err
}
