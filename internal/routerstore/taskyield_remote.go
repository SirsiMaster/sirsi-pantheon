package routerstore

func (rs *RemoteStore) YieldTaskLease(agent, taskID, token, disposition, evidenceRef, dependency string) error {
	return rs.call("YieldTaskLease", []any{agent, taskID, token, disposition, evidenceRef, dependency})
}
