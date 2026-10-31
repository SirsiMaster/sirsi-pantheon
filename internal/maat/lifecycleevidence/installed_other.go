//go:build !darwin

package lifecycleevidence

import "fmt"

func NewInstalledRouter(ItemReader) (Router, error) {
	return Router{}, fmt.Errorf("installed lifecycle acceptance is available only from the verified canonical macOS package")
}
