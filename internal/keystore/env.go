package keystore

import "os"

type OSEnv struct{}

func (OSEnv) Get(key string) string {
	return os.Getenv(key)
}
