//go:build !darwin || !cgo

package keystore

type SystemKeychain struct{}

func (SystemKeychain) Get(string, string) (string, error) {
	return "", ErrUnavailable
}

func (SystemKeychain) Set(string, string, string) error {
	return ErrUnavailable
}

func (SystemKeychain) Delete(string, string) error {
	return ErrUnavailable
}
