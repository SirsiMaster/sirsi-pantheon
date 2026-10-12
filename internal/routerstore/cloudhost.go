package routerstore

import "regexp"

var cloudExecutorHostPattern = regexp.MustCompile(`^cloud-[a-z0-9][a-z0-9-]{0,120}$`)

func validCloudExecutorHost(host string) bool { return cloudExecutorHostPattern.MatchString(host) }
