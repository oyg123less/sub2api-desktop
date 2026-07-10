//go:build !windows

package codexcfg

import "os"

func replaceFile(source, target string) error { return os.Rename(source, target) }
