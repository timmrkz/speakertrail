//go:build darwin

package keys

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

// The item the key is kept in: a generic password, found by its service
// and its account.
static CFMutableDictionaryRef st_item(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

// Whether the item is there. Only its attributes are asked for, never the
// secret, so nothing is put on screen.
static int st_has(const char *service, const char *account) {
	CFMutableDictionaryRef q = st_item(service, account);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	if (found != NULL) CFRelease(found);
	CFRelease(q);
	return (int)status;
}

// The secret, copied into memory the caller frees.
static int st_get(const char *service, const char *account, char **out, long *length) {
	CFMutableDictionaryRef q = st_item(service, account);
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

// Keeps the secret. An item that is there has its secret replaced, and
// one that is not is added, with the access list macOS gives an item an
// app makes: that app, and nothing else without asking.
static int st_set(const char *service, const char *account, const char *secret, long length) {
	CFMutableDictionaryRef q = st_item(service, account);
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, length);
	CFMutableDictionaryRef change = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(change, kSecValueData, data);
	OSStatus status = SecItemUpdate(q, change);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, data);
		status = SecItemAdd(q, NULL);
	}
	CFRelease(change);
	CFRelease(data);
	CFRelease(q);
	return (int)status;
}

static int st_remove(const char *service, const char *account) {
	CFMutableDictionaryRef q = st_item(service, account);
	OSStatus status = SecItemDelete(q);
	CFRelease(q);
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

// keychain keeps the key through the Security framework.
type keychain struct{}

func names(do func(service, account *C.char)) {
	service, acct := C.CString(Service), C.CString(account)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(acct))
	do(service, acct)
}

func (keychain) has() bool {
	found := false
	names(func(service, account *C.char) { found = C.st_has(service, account) == 0 })
	return found
}

func (keychain) get() (string, bool) {
	secret, ok := "", false
	names(func(service, account *C.char) {
		var out *C.char
		var length C.long
		if C.st_get(service, account, &out, &length) != 0 {
			return
		}
		defer C.st_forget(out, length)
		secret, ok = C.GoStringN(out, C.int(length)), true
	})
	return secret, ok
}

func (keychain) set(secret string) error {
	var status C.int
	names(func(service, account *C.char) {
		value := C.CString(secret)
		defer C.st_forget(value, C.long(len(secret)))
		status = C.st_set(service, account, value, C.long(len(secret)))
	})
	if status != 0 {
		return fmt.Errorf("the keychain did not keep the key, status %d", int(status))
	}
	return nil
}

func (keychain) remove() {
	names(func(service, account *C.char) { C.st_remove(service, account) })
}
