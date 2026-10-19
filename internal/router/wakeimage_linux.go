package router

import "os"

func nativeWakeImageIdentity() (wakeImageIdentity, error) {
	// procfs pins the executable vnode even if its installed name was replaced.
	info, err := os.Stat("/proc/self/exe")
	if err != nil {
		return wakeImageIdentity{}, err
	}
	return wakeFileIdentity(info)
}
