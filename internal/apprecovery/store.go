package apprecovery

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type FileStore struct {
	Root string
}

func (s FileStore) Save(receipt Receipt) error {
	if !targetIDPattern.MatchString(receipt.TargetID) {
		return errors.New("invalid recovery receipt target id")
	}
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Root, ".recovery-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(s.Root, receipt.TargetID+".json"))
}

func (s FileStore) Load(targetID string) (Receipt, error) {
	if !targetIDPattern.MatchString(targetID) {
		return Receipt{}, errors.New("invalid recovery receipt target id")
	}
	data, err := os.ReadFile(filepath.Join(s.Root, targetID+".json"))
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.Schema != "pantheon.app-recovery.v1" || receipt.TargetID != targetID {
		return Receipt{}, errors.New("recovery receipt identity mismatch")
	}
	return receipt, nil
}
