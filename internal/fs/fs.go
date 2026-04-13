package fs

import (
	"os"
	"path/filepath"
)

func EnsureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func WriteFile(path string, data []byte, overwrite bool) error {
	if err := EnsureParent(path); err != nil {
		return err
	}
	flag := os.O_CREATE | os.O_WRONLY
	if overwrite {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_EXCL
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(data)
	return err
}
