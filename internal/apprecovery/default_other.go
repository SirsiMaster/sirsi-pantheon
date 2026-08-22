//go:build !darwin

package apprecovery

func LoadDefaultManager() (*Manager, error) {
	return nil, nil
}
