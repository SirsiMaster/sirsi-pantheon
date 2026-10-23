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

func (r *RemoteStore) MaatDelegation(action string, req schedule.Delegation) (schedule.Delegation, error) {
	var out schedule.Delegation
	err := r.call("MaatDelegation", []any{action, req}, &out)
	return out, err
}
func (r *RemoteStore) MaatGetDelegation(pod string) (*schedule.Delegation, error) {
	var out *schedule.Delegation
	err := r.call("MaatGetDelegation", []any{pod}, &out)
	return out, err
}
func (r *RemoteStore) MaatAuthorityReserve(machine string, req schedule.Reservation) (schedule.ReserveResult, error) {
	var out schedule.ReserveResult
	err := r.call("MaatAuthorityReserve", []any{machine, req}, &out)
	return out, err
}
func (r *RemoteStore) MaatReconcile(machine, pod string, epoch int, rows []schedule.Reservation) (int64, error) {
	var out int64
	err := r.call("MaatReconcile", []any{machine, pod, epoch, rows}, &out)
	return out, err
}
