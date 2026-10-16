//go:build darwin && cgo

package enrollment

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#cgo CFLAGS: -Wno-deprecated-declarations
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/cssmtype.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef zpr_runtime_query(SecKeychainRef keychain, const char *tag) {
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFStringRef label = CFStringCreateWithCString(NULL, tag, kCFStringEncodingUTF8);
    CFDictionarySetValue(query, kSecClass, kSecClassKey);
    CFDictionarySetValue(query, kSecAttrKeyType, kSecAttrKeyTypeRSA);
    CFDictionarySetValue(query, kSecAttrKeyClass, kSecAttrKeyClassPrivate);
    CFDictionarySetValue(query, kSecAttrLabel, label);
    const void *value = keychain;
    CFArrayRef list = CFArrayCreate(NULL, &value, 1, &kCFTypeArrayCallBacks);
    CFDictionarySetValue(query, kSecMatchSearchList, list);
    CFRelease(list);
    CFRelease(label);
    return query;
}

static OSStatus zpr_runtime_find(const char *path, const char *tag, SecKeyRef *key) {
    SecKeychainRef keychain = NULL;
    OSStatus status = SecKeychainOpen(path, &keychain);
    if (status != errSecSuccess) return status;
    CFMutableDictionaryRef query = zpr_runtime_query(keychain, tag);
    CFDictionarySetValue(query, kSecReturnRef, kCFBooleanTrue);
    CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitAll);
    CFTypeRef result = NULL;
    status = SecItemCopyMatching(query, &result);
    if (status == errSecSuccess) {
        if (!result || CFGetTypeID(result) != CFArrayGetTypeID() ||
            CFArrayGetCount((CFArrayRef)result) != 1) status = errSecDuplicateItem;
        else {
            CFTypeRef item = CFArrayGetValueAtIndex((CFArrayRef)result, 0);
            if (CFGetTypeID(item) != SecKeyGetTypeID()) status = errSecDecode;
            else *key = (SecKeyRef)CFRetain(item);
        }
    }
    if (result) CFRelease(result);
    CFRelease(query);
    CFRelease(keychain);
    return status;
}

static OSStatus zpr_runtime_import(const char *path, const char *tag, const void *bytes, size_t size) {
    SecKeyRef existing = NULL;
    OSStatus status = zpr_runtime_find(path, tag, &existing);
    if (status == errSecSuccess) { CFRelease(existing); return errSecDuplicateItem; }
    if (status != errSecItemNotFound) return status;
    SecKeychainRef keychain = NULL;
    status = SecKeychainOpen(path, &keychain);
    if (status != errSecSuccess) return status;
    CFDataRef data = CFDataCreate(NULL, bytes, (CFIndex)size);
    SecExternalFormat format = kSecFormatOpenSSL;
    SecExternalItemType type = kSecItemTypePrivateKey;
    SecItemImportExportKeyParameters params = {0};
    params.version = SEC_KEY_IMPORT_EXPORT_PARAMS_VERSION;
    params.flags = kSecKeyImportOnlyOne;
    int permanent = CSSM_KEYATTR_PERMANENT;
    int sensitive = CSSM_KEYATTR_SENSITIVE;
    CFNumberRef permanent_value = CFNumberCreate(NULL, kCFNumberIntType, &permanent);
    CFNumberRef sensitive_value = CFNumberCreate(NULL, kCFNumberIntType, &sensitive);
    const void *values[] = {permanent_value, sensitive_value};
    params.keyAttributes = CFArrayCreate(NULL, values, 2, &kCFTypeArrayCallBacks);
    CFStringRef label = CFStringCreateWithCString(NULL, tag, kCFStringEncodingUTF8);
    CFArrayRef items = NULL;
    status = SecItemImport(data, label, &format, &type, 0, &params, keychain, &items);
    if (status == errSecSuccess) {
        if (!items || CFArrayGetCount(items) != 1) status = errSecDecode;
        else {
            SecKeychainAttribute attr = {kSecKeyPrintName, (UInt32)strlen(tag), (void *)tag};
            SecKeychainAttributeList attrs = {1, &attr};
            status = SecKeychainItemModifyAttributesAndData(
                (SecKeychainItemRef)CFArrayGetValueAtIndex(items, 0), &attrs, 0, NULL);
        }
    }
    if (items) CFRelease(items);
    CFRelease(label);
    CFRelease(params.keyAttributes);
    CFRelease(permanent_value);
    CFRelease(sensitive_value);
    CFRelease(data);
    CFRelease(keychain);
    return status;
}

static OSStatus zpr_runtime_bytes(const char *path, const char *tag, const void *digest,
    size_t digest_size, void **bytes, size_t *size) {
    SecKeyRef key = NULL;
    OSStatus status = zpr_runtime_find(path, tag, &key);
    if (status != errSecSuccess) return status;
    CFErrorRef error = NULL;
    CFDataRef result = NULL;
    if (digest) {
        CFDataRef input = CFDataCreate(NULL, digest, (CFIndex)digest_size);
        result = SecKeyCreateSignature(key, kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256, input, &error);
        CFRelease(input);
    } else {
        SecKeyRef public_key = SecKeyCopyPublicKey(key);
        if (public_key) {
            result = SecKeyCopyExternalRepresentation(public_key, &error);
            CFRelease(public_key);
        }
    }
    if (!result) status = error ? (OSStatus)CFErrorGetCode(error) : errSecDecode;
    else {
        CFIndex length = CFDataGetLength(result);
        if (length <= 0 || length > 8192) status = errSecDecode;
        else {
            *bytes = malloc((size_t)length);
            if (!*bytes) status = errSecAllocate;
            else {
                memcpy(*bytes, CFDataGetBytePtr(result), (size_t)length);
                *size = (size_t)length;
            }
        }
        CFRelease(result);
    }
    if (error) CFRelease(error);
    CFRelease(key);
    return status;
}

static OSStatus zpr_runtime_nonextractable(const char *path, const char *tag) {
    SecKeyRef key = NULL;
    OSStatus status = zpr_runtime_find(path, tag, &key);
    if (status != errSecSuccess) return status;
    CFDictionaryRef attrs = SecKeyCopyAttributes(key);
    if (!attrs || CFDictionaryGetValue(attrs, kSecAttrIsExtractable) != kCFBooleanFalse)
        status = errSecAuthFailed;
    if (attrs) CFRelease(attrs);
    CFRelease(key);
    return status;
}

// Inspect the legacy Keychain's actual key header, not just requested attributes.
static OSStatus zpr_native_key_policy(SecKeyRef key) {
    const CSSM_KEY *cssm = NULL;
    OSStatus status = SecKeyGetCSSMKey(key, &cssm);
    if (status != errSecSuccess) return status;
    if (!cssm || cssm->KeyHeader.AlgorithmId != CSSM_ALGID_RSA ||
        cssm->KeyHeader.LogicalKeySizeInBits != 3072 ||
        !(cssm->KeyHeader.KeyAttr & CSSM_KEYATTR_PERMANENT) ||
        !(cssm->KeyHeader.KeyAttr & CSSM_KEYATTR_SENSITIVE) ||
        !(cssm->KeyHeader.KeyAttr & CSSM_KEYATTR_ALWAYS_SENSITIVE) ||
        !(cssm->KeyHeader.KeyAttr & CSSM_KEYATTR_NEVER_EXTRACTABLE) ||
        (cssm->KeyHeader.KeyAttr & CSSM_KEYATTR_EXTRACTABLE))
        return errSecAuthFailed;
    CFDictionaryRef attrs = SecKeyCopyAttributes(key);
    if (!attrs || CFDictionaryGetValue(attrs, kSecAttrIsExtractable) != kCFBooleanFalse)
        status = errSecAuthFailed;
    if (attrs) CFRelease(attrs);
    return status;
}

static OSStatus zpr_native_bytes(const char *path, const char *tag, const void *digest,
    size_t digest_size, void **bytes, size_t *size) {
    // The default creator ACL is not sufficient authorization for signing.
    if (digest) return errSecAuthFailed;
    SecKeyRef key = NULL;
    OSStatus status = zpr_runtime_find(path, tag, &key);
    if (status != errSecSuccess) return status;
    status = zpr_native_key_policy(key);
    CFDataRef result = NULL;
    CFErrorRef error = NULL;
    if (status == errSecSuccess) {
        if (digest) {
            CFDataRef input = CFDataCreate(NULL, digest, (CFIndex)digest_size);
            result = SecKeyCreateSignature(key, kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256, input, &error);
            CFRelease(input);
        } else {
            SecKeyRef public_key = SecKeyCopyPublicKey(key);
            if (public_key) {
                result = SecKeyCopyExternalRepresentation(public_key, &error);
                CFRelease(public_key);
            }
        }
        if (!result) status = error ? (OSStatus)CFErrorGetCode(error) : errSecDecode;
    }
    if (result) {
        CFIndex length = CFDataGetLength(result);
        if (length <= 0 || length > 8192) status = errSecDecode;
        else {
            *bytes = malloc((size_t)length);
            if (!*bytes) status = errSecAllocate;
            else {
                memcpy(*bytes, CFDataGetBytePtr(result), (size_t)length);
                *size = (size_t)length;
            }
        }
        CFRelease(result);
    }
    if (error) CFRelease(error);
    CFRelease(key);
    return status;
}
*/
import "C"

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// runtimeKeyVault is not exposed to the browser or a network listener.
// Its path must be supplied by trusted helper configuration, never a device.
type runtimeKeyVault struct{ path string }

var errRuntimeKeyAbsent = errors.New("runtime Keychain key absent")
var errNativeEndpointAccessControlUnavailable = errors.New("native endpoint key access is disabled because legacy Keychain ACLs cannot enforce helper-only access")

// SecAccess trusted applications are allowed to skip confirmation; they do
// not exclude other applications from requesting access. With no supported,
// trusted helper-only policy, do not create a key using creator/default ACLs.
func (v runtimeKeyVault) generateNative(tag string) error {
	return errNativeEndpointAccessControlUnavailable
}

func (v runtimeKeyVault) nativeBytes(tag string, digest []byte) ([]byte, error) {
	// Public lookup and signing share this gate: returning a public key here
	// would qualify a native key whose access metadata is not enforceable.
	return nil, errNativeEndpointAccessControlUnavailable
}

type runtimeVaultSigner struct {
	vault runtimeKeyVault
	tag   string
	key   *rsa.PublicKey
}

func (v runtimeKeyVault) validate(tag string) error {
	if !filepath.IsAbs(v.path) || filepath.Clean(v.path) != v.path ||
		len(tag) != len("com.zpr.runtime.")+64 || tag[:len("com.zpr.runtime.")] != "com.zpr.runtime." ||
		!validFingerprint(tag[len("com.zpr.runtime."):]) {
		return ErrInvalid
	}
	info, err := os.Lstat(v.path)
	if err != nil {
		return fmt.Errorf("runtime Keychain path unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("runtime Keychain must be an explicitly configured regular file, not a symlink")
	}
	return nil
}

func (v runtimeKeyVault) validateNativeEndpoint(tag string) error {
	if err := v.validate(tag); err != nil {
		return err
	}
	root, err := openIdentityDirectory(filepath.Dir(v.path), false)
	if err != nil {
		return fmt.Errorf("runtime Keychain parent must be protected: %w", err)
	}
	defer root.Close()
	name := filepath.Base(v.path)
	entry, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("runtime Keychain path unavailable: %w", err)
	}
	if !entry.Mode().IsRegular() || !privateOwned(entry) {
		return errors.New("runtime Keychain must be a private owner-only regular file")
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	current, currentErr := root.Lstat(name)
	if !ok || stat.Nlink != 1 || !info.Mode().IsRegular() || !privateOwned(info) ||
		currentErr != nil || !current.Mode().IsRegular() || !privateOwned(current) ||
		!os.SameFile(entry, info) || !os.SameFile(current, info) {
		return errors.New("runtime Keychain path is unsafe or changed while opening")
	}
	return nil
}

func (v runtimeKeyVault) lookup(tag string) (*runtimeVaultSigner, error) {
	if err := v.validate(tag); err != nil {
		return nil, err
	}
	path, label := C.CString(v.path), C.CString(tag)
	defer C.free(unsafe.Pointer(path))
	defer C.free(unsafe.Pointer(label))
	var data unsafe.Pointer
	var size C.size_t
	status := C.zpr_runtime_bytes(path, label, nil, 0, &data, &size)
	defer C.free(data)
	if status == C.errSecItemNotFound {
		return nil, errRuntimeKeyAbsent
	}
	if err := keychainError("runtime public key lookup", status); err != nil {
		return nil, err
	}
	if err := keychainError("runtime non-extractable policy", C.zpr_runtime_nonextractable(path, label)); err != nil {
		return nil, err
	}
	public, err := x509.ParsePKCS1PublicKey(C.GoBytes(data, C.int(size)))
	if err != nil {
		return nil, fmt.Errorf("parse runtime Keychain public key: %w", err)
	}
	return &runtimeVaultSigner{vault: v, tag: tag, key: public}, nil
}

func (v runtimeKeyVault) importKey(tag string, key *rsa.PrivateKey) error {
	if err := v.validate(tag); err != nil {
		return err
	}
	if key == nil || key.N == nil || key.N.BitLen() != 3072 || key.E != 65537 || key.Validate() != nil {
		return ErrInvalid
	}
	data := x509.MarshalPKCS1PrivateKey(key)
	defer clear(data)
	path, label := C.CString(v.path), C.CString(tag)
	defer C.free(unsafe.Pointer(path))
	defer C.free(unsafe.Pointer(label))
	return keychainError("runtime import", C.zpr_runtime_import(path, label, unsafe.Pointer(&data[0]), C.size_t(len(data))))
}

func (s *runtimeVaultSigner) Public() crypto.PublicKey { return s.key }

// Sign preserves the existing explicit-import runtime path. The new native
// endpoint vault uses nativeEndpointSigner and is denied by nativeBytes below.
func (s *runtimeVaultSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil || opts.HashFunc() != crypto.SHA256 || len(digest) != sha256.Size {
		return nil, errors.New("runtime Keychain signer requires a SHA-256 PKCS#1 v1.5 digest")
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return nil, errors.New("runtime Keychain signer does not support RSA-PSS")
	}
	path, label := C.CString(s.vault.path), C.CString(s.tag)
	defer C.free(unsafe.Pointer(path))
	defer C.free(unsafe.Pointer(label))
	var data unsafe.Pointer
	var size C.size_t
	status := C.zpr_runtime_bytes(path, label, unsafe.Pointer(&digest[0]), C.size_t(len(digest)), &data, &size)
	defer C.free(data)
	if err := keychainError("runtime signing", status); err != nil {
		return nil, err
	}
	return C.GoBytes(data, C.int(size)), nil
}
