//go:build darwin && cgo

package enrollment

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#cgo CFLAGS: -Wno-deprecated-declarations
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef zpr_keychain_query(void) {
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
    CFDictionarySetValue(query, kSecAttrService, CFSTR("com.zpr.enrollment.development"));
    return query;
}

static OSStatus zpr_keychain_open(const char *path, SecKeychainRef *keychain) {
    if (path && path[0]) return SecKeychainOpen(path, keychain);
    return SecKeychainCopyDefault(keychain);
}

static OSStatus zpr_copy_keychain_data(CFTypeRef result, void **data, size_t *size) {
    if (!result || CFGetTypeID(result) != CFDataGetTypeID()) return errSecDecode;
    CFIndex length = CFDataGetLength((CFDataRef)result);
    if (length <= 0 || length > 65536) return errSecDecode;
    *data = malloc((size_t)length);
    if (!*data) return errSecAllocate;
    memcpy(*data, CFDataGetBytePtr((CFDataRef)result), (size_t)length);
    *size = (size_t)length;
    return errSecSuccess;
}

static void zpr_free_keychain_data(void *data, size_t size) {
    if (data) {
        memset(data, 0, size);
        free(data);
    }
}

static OSStatus zpr_keychain_add(const char *path, const char *account,
    const void *bytes, size_t size, void **reference, size_t *reference_size) {
    SecKeychainRef keychain = NULL;
    OSStatus status = zpr_keychain_open(path, &keychain);
    if (status != errSecSuccess) return status;
    CFMutableDictionaryRef query = zpr_keychain_query();
    CFStringRef name = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
    CFDataRef data = CFDataCreate(NULL, bytes, (CFIndex)size);
    CFDictionarySetValue(query, kSecAttrAccount, name);
    CFDictionarySetValue(query, kSecAttrLabel, CFSTR("ZPR development enrollment identity"));
    CFDictionarySetValue(query, kSecValueData, data);
    CFDictionarySetValue(query, kSecUseKeychain, keychain);
    CFDictionarySetValue(query, kSecReturnPersistentRef, kCFBooleanTrue);
    CFTypeRef result = NULL;
    status = SecItemAdd(query, &result);
    if (status == errSecSuccess) status = zpr_copy_keychain_data(result, reference, reference_size);
    if (result) CFRelease(result);
    CFRelease(data);
    CFRelease(name);
    CFRelease(query);
    CFRelease(keychain);
    return status;
}

static CFMutableDictionaryRef zpr_keychain_reference_query(const void *bytes, size_t size) {
    CFMutableDictionaryRef query = zpr_keychain_query();
    CFDataRef reference = CFDataCreate(NULL, bytes, (CFIndex)size);
    const void *value = reference;
    CFArrayRef items = CFArrayCreate(NULL, &value, 1, &kCFTypeArrayCallBacks);
    CFDictionarySetValue(query, kSecMatchItemList, items);
    CFRelease(items);
    CFRelease(reference);
    return query;
}

static OSStatus zpr_keychain_read(const char *path, const void *reference, size_t reference_size,
    void **bytes, size_t *size) {
    SecKeychainRef keychain = NULL;
    OSStatus status = zpr_keychain_open(path, &keychain);
    if (status != errSecSuccess) return status;
    CFMutableDictionaryRef query = zpr_keychain_reference_query(reference, reference_size);
    const void *keychain_value = keychain;
    CFArrayRef keychains = CFArrayCreate(NULL, &keychain_value, 1, &kCFTypeArrayCallBacks);
    CFDictionarySetValue(query, kSecMatchSearchList, keychains);
    CFRelease(keychains);
    CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
    CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
    CFTypeRef result = NULL;
    status = SecItemCopyMatching(query, &result);
    if (status == errSecSuccess) status = zpr_copy_keychain_data(result, bytes, size);
    if (result) CFRelease(result);
    CFRelease(query);
    CFRelease(keychain);
    return status;
}

static OSStatus zpr_keychain_remove(const char *path, const void *reference, size_t reference_size) {
    SecKeychainRef keychain = NULL;
    OSStatus status = zpr_keychain_open(path, &keychain);
    if (status != errSecSuccess) return status;
    CFMutableDictionaryRef query = zpr_keychain_reference_query(reference, reference_size);
    const void *keychain_value = keychain;
    CFArrayRef keychains = CFArrayCreate(NULL, &keychain_value, 1, &kCFTypeArrayCallBacks);
    CFDictionarySetValue(query, kSecMatchSearchList, keychains);
    CFRelease(keychains);
    status = SecItemDelete(query);
    CFRelease(query);
    CFRelease(keychain);
    return status;
}

static OSStatus zpr_keychain_create_test(const char *path, const char *password) {
    SecKeychainRef keychain = NULL;
    OSStatus status = SecKeychainCreate(path, (UInt32)strlen(password), password, false, NULL, &keychain);
    if (keychain) CFRelease(keychain);
    return status;
}

static OSStatus zpr_keychain_delete_test(const char *path) {
    SecKeychainRef keychain = NULL;
    OSStatus status = SecKeychainOpen(path, &keychain);
    if (status == errSecSuccess) {
        status = SecKeychainDelete(keychain);
        CFRelease(keychain);
    }
    return status;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// An empty path selects the desktop user's configured default Keychain.
type macKeychain struct{ path string }

func keychainAvailable() bool { return true }

func keychainError(operation string, status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	return fmt.Errorf("macOS Keychain %s failed (OSStatus %d); unlock/authorize the Keychain or seek administrator recovery", operation, int32(status))
}

func (k macKeychain) add(account string, data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > 16384 {
		return nil, errors.New("invalid Keychain identity size")
	}
	path, name := C.CString(k.path), C.CString(account)
	defer C.free(unsafe.Pointer(path))
	defer C.free(unsafe.Pointer(name))
	var reference unsafe.Pointer
	var size C.size_t
	status := C.zpr_keychain_add(path, name, unsafe.Pointer(&data[0]), C.size_t(len(data)), &reference, &size)
	defer C.zpr_free_keychain_data(reference, size)
	if err := keychainError("save", status); err != nil {
		return nil, err
	}
	return C.GoBytes(reference, C.int(size)), nil
}

func (k macKeychain) read(reference []byte) ([]byte, error) {
	if len(reference) == 0 || len(reference) > 4096 {
		return nil, errors.New("invalid Keychain reference")
	}
	var data unsafe.Pointer
	var size C.size_t
	path := C.CString(k.path)
	defer C.free(unsafe.Pointer(path))
	status := C.zpr_keychain_read(path, unsafe.Pointer(&reference[0]), C.size_t(len(reference)), &data, &size)
	defer C.zpr_free_keychain_data(data, size)
	if err := keychainError("read", status); err != nil {
		return nil, err
	}
	return C.GoBytes(data, C.int(size)), nil
}

func (k macKeychain) remove(reference []byte) error {
	if len(reference) == 0 || len(reference) > 4096 {
		return errors.New("invalid Keychain reference")
	}
	path := C.CString(k.path)
	defer C.free(unsafe.Pointer(path))
	return keychainError("remove", C.zpr_keychain_remove(path, unsafe.Pointer(&reference[0]), C.size_t(len(reference))))
}

func createTestKeychain(path, password string) error {
	name, secret := C.CString(path), C.CString(password)
	defer C.free(unsafe.Pointer(name))
	defer C.zpr_free_keychain_data(unsafe.Pointer(secret), C.size_t(len(password)+1))
	return keychainError("create isolated test keychain", C.zpr_keychain_create_test(name, secret))
}

func deleteTestKeychain(path string) error {
	name := C.CString(path)
	defer C.free(unsafe.Pointer(name))
	return keychainError("delete isolated test keychain", C.zpr_keychain_delete_test(name))
}
