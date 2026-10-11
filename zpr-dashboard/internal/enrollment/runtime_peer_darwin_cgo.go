//go:build darwin && cgo

package enrollment

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation -lbsm
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <bsm/libbsm.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <stdlib.h>
#include <errno.h>

static OSStatus zpr_runtime_verify_peer(int fd, const char *text, uid_t expected_uid) {
    audit_token_t token;
    socklen_t size = sizeof(token);
    if (getsockopt(fd, SOL_LOCAL, LOCAL_PEERTOKEN, &token, &size) != 0 ||
        size != sizeof(token)) return errSecParam;
    if (audit_token_to_euid(token) != expected_uid) return errSecAuthFailed;
    CFDataRef audit = CFDataCreate(NULL, (const UInt8 *)&token, sizeof(token));
    CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(NULL, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(attrs, kSecGuestAttributeAudit, audit);
    SecCodeRef code = NULL;
    OSStatus status = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &code);
    CFRelease(attrs);
    CFRelease(audit);
    if (status != errSecSuccess) return status;
    CFStringRef string = CFStringCreateWithCString(NULL, text, kCFStringEncodingUTF8);
    SecRequirementRef requirement = NULL;
    status = SecRequirementCreateWithString(string, kSecCSDefaultFlags, &requirement);
    if (status == errSecSuccess)
        status = SecCodeCheckValidityWithErrors(code, kSecCSStrictValidate, requirement, NULL);
    if (requirement) CFRelease(requirement);
    CFRelease(string);
    CFRelease(code);
    return status;
}

// Used only by isolated tests to pin their own ad-hoc executable.
static OSStatus zpr_runtime_test_requirement(char **text) {
    SecCodeRef code = NULL;
    OSStatus status = SecCodeCopySelf(kSecCSDefaultFlags, &code);
    if (status != errSecSuccess) return status;
    SecRequirementRef requirement = NULL;
    status = SecCodeCopyDesignatedRequirement(code, kSecCSDefaultFlags, &requirement);
    CFStringRef string = NULL;
    if (status == errSecSuccess)
        status = SecRequirementCopyString(requirement, kSecCSDefaultFlags, &string);
    if (status == errSecSuccess) {
        *text = calloc(8193, 1);
        if (!*text) status = errSecAllocate;
        else if (!CFStringGetCString(string, *text, 8193, kCFStringEncodingUTF8)) status = errSecDecode;
    }
    if (string) CFRelease(string);
    if (requirement) CFRelease(requirement);
    CFRelease(code);
    return status;
}
*/
import "C"

import (
	"errors"
	"net"
	"strings"
	"unsafe"
)

// The requirement and UID must come from trusted helper configuration.
// They must never be selected by a browser, socket message, or the peer.
func verifyRuntimeConnection(connection *net.UnixConn, requirement string, expectedUID int) error {
	if connection == nil || len(requirement) == 0 || len(requirement) > 8192 ||
		strings.ContainsRune(requirement, 0) || expectedUID < 0 || uint64(expectedUID) > 1<<32-1 {
		return ErrInvalid
	}
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	text := C.CString(requirement)
	defer C.free(unsafe.Pointer(text))
	var status C.OSStatus
	if err := raw.Control(func(fd uintptr) {
		status = C.zpr_runtime_verify_peer(C.int(fd), text, C.uid_t(expectedUID))
	}); err != nil {
		return err
	}
	if status != C.errSecSuccess {
		return errors.New("runtime IPC peer code signature, audit token or user identity rejected")
	}
	return nil
}

func currentRuntimeTestRequirement() (string, error) {
	var text *C.char
	status := C.zpr_runtime_test_requirement(&text)
	defer C.free(unsafe.Pointer(text))
	if err := keychainError("isolated test code requirement", status); err != nil {
		return "", err
	}
	return C.GoString(text), nil
}
