//go:build darwin && cgo

package keystore

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>

static OSStatus kcFindPassword(const char *service, UInt32 serviceLen, const char *account, UInt32 accountLen, UInt32 *passwordLen, void **passwordData) {
	return SecKeychainFindGenericPassword(NULL, serviceLen, service, accountLen, account, passwordLen, passwordData, NULL);
}

static OSStatus kcAddPassword(const char *service, UInt32 serviceLen, const char *account, UInt32 accountLen, UInt32 passwordLen, const void *passwordData, SecKeychainItemRef *item) {
	return SecKeychainAddGenericPassword(NULL, serviceLen, service, accountLen, account, passwordLen, passwordData, item);
}

static OSStatus kcFindItem(const char *service, UInt32 serviceLen, const char *account, UInt32 accountLen, SecKeychainItemRef *item) {
	return SecKeychainFindGenericPassword(NULL, serviceLen, service, accountLen, account, NULL, NULL, item);
}

static OSStatus kcModifyPassword(SecKeychainItemRef item, UInt32 passwordLen, const void *passwordData) {
	return SecKeychainItemModifyAttributesAndData(item, NULL, passwordLen, passwordData);
}

static OSStatus kcDeleteItem(SecKeychainItemRef item) {
	return SecKeychainItemDelete(item);
}

static OSStatus kcFreeContent(void *passwordData) {
	return SecKeychainItemFreeContent(NULL, passwordData);
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

type SystemKeychain struct{}

func (SystemKeychain) Get(service, account string) (string, error) {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return "", ErrUnavailable
	}

	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))

	var passwordLength C.UInt32
	var passwordData unsafe.Pointer
	status := C.kcFindPassword(
		cService,
		C.UInt32(len(service)),
		cAccount,
		C.UInt32(len(account)),
		&passwordLength,
		&passwordData,
	)
	if status != C.errSecSuccess {
		return "", mapKeychainStatus(status)
	}
	defer C.kcFreeContent(passwordData)
	if passwordLength == 0 || passwordData == nil {
		return "", ErrNotFound
	}
	return strings.TrimSpace(C.GoStringN((*C.char)(passwordData), C.int(passwordLength))), nil
}

func (SystemKeychain) Set(service, account, value string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	value = strings.TrimSpace(value)
	if service == "" || account == "" || value == "" {
		return ErrUnavailable
	}

	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))
	cValue := C.CBytes([]byte(value))
	defer C.free(cValue)

	var item C.SecKeychainItemRef
	status := C.kcAddPassword(
		cService,
		C.UInt32(len(service)),
		cAccount,
		C.UInt32(len(account)),
		C.UInt32(len(value)),
		cValue,
		&item,
	)
	if item != C.SecKeychainItemRef(0) {
		C.CFRelease(C.CFTypeRef(item))
	}
	if status == C.errSecSuccess {
		return nil
	}
	if status != C.errSecDuplicateItem {
		return mapKeychainStatus(status)
	}
	return updateExistingPassword(service, account, value)
}

func (SystemKeychain) Delete(service, account string) error {
	service = strings.TrimSpace(service)
	account = strings.TrimSpace(account)
	if service == "" || account == "" {
		return ErrUnavailable
	}

	item, err := findPasswordItem(service, account)
	if err != nil {
		return err
	}
	defer C.CFRelease(C.CFTypeRef(item))

	status := C.kcDeleteItem(item)
	if status != C.errSecSuccess {
		return mapKeychainStatus(status)
	}
	return nil
}

func updateExistingPassword(service, account, value string) error {
	item, err := findPasswordItem(service, account)
	if err != nil {
		return err
	}
	defer C.CFRelease(C.CFTypeRef(item))

	cValue := C.CBytes([]byte(value))
	defer C.free(cValue)

	status := C.kcModifyPassword(item, C.UInt32(len(value)), cValue)
	if status != C.errSecSuccess {
		return mapKeychainStatus(status)
	}
	return nil
}

func findPasswordItem(service, account string) (C.SecKeychainItemRef, error) {
	cService := C.CString(service)
	defer C.free(unsafe.Pointer(cService))
	cAccount := C.CString(account)
	defer C.free(unsafe.Pointer(cAccount))

	var item C.SecKeychainItemRef
	status := C.kcFindItem(
		cService,
		C.UInt32(len(service)),
		cAccount,
		C.UInt32(len(account)),
		&item,
	)
	if status != C.errSecSuccess {
		return C.SecKeychainItemRef(0), mapKeychainStatus(status)
	}
	return item, nil
}

func mapKeychainStatus(status C.OSStatus) error {
	switch status {
	case C.errSecItemNotFound:
		return ErrNotFound
	case C.errSecNoDefaultKeychain, C.errSecInteractionNotAllowed, C.errSecAuthFailed:
		return ErrUnavailable
	default:
		return fmt.Errorf("%w: macOS Security.framework status %d", ErrUnavailable, int(status))
	}
}
