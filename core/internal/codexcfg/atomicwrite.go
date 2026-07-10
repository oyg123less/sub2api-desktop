package codexcfg

import (
	"fmt"
	"os"
	"path/filepath"
)

func atomicWriteFile(path string, data []byte, mode os.FileMode) (resultErr error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".amber-write-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if resultErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
