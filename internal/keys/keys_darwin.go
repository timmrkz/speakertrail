//go:build darwin

package keys

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
// The tests make a keychain of their own with calls Apple marks
// deprecated without a replacement for it.
#cgo CFLAGS: -Wno-deprecated-declarations
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

// The item the key is kept in: a generic password, found by its service
// and its account. kc is the keychain to look in, NULL for the person's
// own, a keychain of its own in the tests.
static CFMutableDictionaryRef st_item(SecKeychainRef kc, const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	if (kc != NULL) {
		CFArrayRef list = CFArrayCreate(NULL, (const void **)&kc, 1, &kCFTypeArrayCallBacks);
		CFDictionarySetValue(q, kSecMatchSearchList, list);
		CFRelease(list);
	}
	CFRelease(s);
	CFRelease(a);
	return q;
}

// Whether the item is there. Only its attributes are asked for, never the
// secret, so nothing is put on screen.
static int st_has(SecKeychainRef kc, const char *service, const char *account) {
	CFMutableDictionaryRef q = st_item(kc, service, account);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	if (found != NULL) CFRelease(found);
	CFRelease(q);
	return (int)status;
}

// The secret, copied into memory the caller frees.
static int st_get(SecKeychainRef kc, const char *service, const char *account, char **out, long *length) {
	CFMutableDictionaryRef q = st_item(kc, service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	CFRelease(q);
	if (status != errSecSuccess) return (int)status;
	if (found == NULL || CFGetTypeID(found) != CFDataGetTypeID()) {
		if (found != NULL) CFRelease(found);
		return (int)errSecItemNotFound;
	}
	CFDataRef data = (CFDataRef)found;
	*length = CFDataGetLength(data);
	*out = malloc(*length + 1);
	memcpy(*out, CFDataGetBytePtr(data), *length);
	CFRelease(found);
	return 0;
}

static int st_remove(SecKeychainRef kc, const char *service, const char *account) {
	CFMutableDictionaryRef q = st_item(kc, service, account);
	OSStatus status = SecItemDelete(q);
	CFRelease(q);
	return (int)status;
}

// For the tests: a keychain of its own at path, unlocked, with every box
// of macOS turned off, so a read that would ask fails at once instead.
static OSStatus st_test_keychain(const char *path, SecKeychainRef *out) {
	SecKeychainSetUserInteractionAllowed(false);
	return SecKeychainCreate(path, 4, "test", false, NULL, out);
}

static void st_test_keychain_drop(SecKeychainRef kc) {
	SecKeychainDelete(kc);
	CFRelease(kc);
	SecKeychainSetUserInteractionAllowed(true);
}

// For the tests: keeps the secret the way builds before this one did.
static int st_test_set(SecKeychainRef kc, const char *service, const char *account, const char *secret, long length) {
	CFMutableDictionaryRef add = st_item(NULL, service, account);
	CFDictionarySetValue(add, kSecUseKeychain, kc);
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, length);
	CFDictionarySetValue(add, kSecValueData, data);
	OSStatus status = SecItemAdd(add, NULL);
	CFRelease(data);
	CFRelease(add);
	return (int)status;
}

static void st_forget(char *secret, long length) {
	if (secret == NULL) return;
	memset(secret, 0, length);
	free(secret);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func system() vault { return keychain{} }

// keychain is where builds before this one kept the key, read through
// the Security framework, in the person's own keychain, or in kc when the
// tests give one.
type keychain struct{ kc C.SecKeychainRef }

func names(do func(service, account *C.char)) {
	service, acct := C.CString(Service), C.CString(account)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(acct))
	do(service, acct)
}

func (k keychain) has() bool {
	found := false
	names(func(service, account *C.char) { found = C.st_has(k.kc, service, account) == 0 })
	return found
}

func (k keychain) get() (string, int) {
	secret, status := "", 0
	names(func(service, account *C.char) {
		var out *C.char
		var length C.long
		if status = int(C.st_get(k.kc, service, account, &out, &length)); status != 0 {
			return
		}
		defer C.st_forget(out, length)
		secret = C.GoStringN(out, C.int(length))
	})
	return secret, status
}

// testKeychain is a keychain of its own at path, for the tests, with
// every box of macOS turned off until drop.
func testKeychain(path string) (k keychain, drop func(), err error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	if status := C.st_test_keychain(p, &k.kc); status != 0 {
		return k, nil, fmt.Errorf("no keychain for the test, status %d", int(status))
	}
	return k, func() { C.st_test_keychain_drop(k.kc) }, nil
}

// put keeps the secret the way builds before this one did, for the
// tests.
func (k keychain) put(secret string) error {
	var status C.int
	names(func(service, account *C.char) {
		value := C.CString(secret)
		defer C.free(unsafe.Pointer(value))
		status = C.st_test_set(k.kc, service, account, value, C.long(len(secret)))
	})
	if status != 0 {
		return fmt.Errorf("status %d", int(status))
	}
	return nil
}

func (k keychain) remove() int {
	status := 0
	names(func(service, account *C.char) { status = int(C.st_remove(k.kc, service, account)) })
	return status
}
