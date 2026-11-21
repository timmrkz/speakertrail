//go:build darwin

package keys

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
// SecAccess and SecACL are the only way to set who may read an item of
// the login keychain, and Apple marks them deprecated without a
// replacement for it.
#cgo CFLAGS: -Wno-deprecated-declarations
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

// Whether every entry of access that allows reading the secret allows it
// to any app, so that macOS asks only through the item's partition list.
// 1 when it does, 0 when an entry names apps.
static int st_decrypt_open(SecAccessRef access) {
	CFArrayRef acls = SecAccessCopyMatchingACLList(access, kSecACLAuthorizationDecrypt);
	if (acls == NULL) return 0;
	int open = CFArrayGetCount(acls) > 0;
	for (CFIndex i = 0; i < CFArrayGetCount(acls); i++) {
		CFArrayRef apps = NULL;
		CFStringRef description = NULL;
		SecKeychainPromptSelector selector = 0;
		if (SecACLCopyContents((SecACLRef)CFArrayGetValueAtIndex(acls, i), &apps, &description, &selector) != errSecSuccess
			|| apps != NULL) {
			open = 0;
		}
		if (apps != NULL) CFRelease(apps);
		if (description != NULL) CFRelease(description);
	}
	CFRelease(acls);
	return open;
}

// The access a new item gets. macOS checks a read twice, and each check
// that fails asks for the password in a box of its own, one after the
// other. The first is the item's list of trusted apps, the second its
// partition list. An ad hoc signed build is never on either until the
// person picks Always Allow, so a new build was asked twice. Here the
// entry that allows reading the secret allows it to any app, keeping its
// prompt, so the first check passes and only the partition list asks:
// one box, with Always Allow for that build. Another program reading it
// is still asked, by the partition list, which holds only the builds the
// person allowed. See securityd's acls.cpp, validatePartition.
static OSStatus st_access(const char *service, SecAccessRef *out) {
	CFStringRef name = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	SecAccessRef access = NULL;
	OSStatus status = SecAccessCreate(name, NULL, &access);
	if (status != errSecSuccess) {
		CFRelease(name);
		return status;
	}
	CFArrayRef acls = SecAccessCopyMatchingACLList(access, kSecACLAuthorizationDecrypt);
	if (acls == NULL || CFArrayGetCount(acls) == 0) status = errSecInternalComponent;
	for (CFIndex i = 0; acls != NULL && status == errSecSuccess && i < CFArrayGetCount(acls); i++) {
		SecACLRef acl = (SecACLRef)CFArrayGetValueAtIndex(acls, i);
		CFArrayRef apps = NULL;
		CFStringRef description = NULL;
		SecKeychainPromptSelector selector = 0;
		status = SecACLCopyContents(acl, &apps, &description, &selector);
		if (status == errSecSuccess) {
			status = SecACLSetContents(acl, NULL, description != NULL ? description : name, selector);
		}
		if (apps != NULL) CFRelease(apps);
		if (description != NULL) CFRelease(description);
	}
	if (acls != NULL) CFRelease(acls);
	CFRelease(name);
	if (status != errSecSuccess) {
		CFRelease(access);
		return status;
	}
	*out = access;
	return errSecSuccess;
}

// Whether the item is kept with that access: 1 when it is, 0 when it is
// not, a status below 0 when it cannot be told. Only the item's access
// list is read, never the secret, so nothing is put on screen.
static int st_open(const char *service, const char *account) {
	CFMutableDictionaryRef q = st_item(service, account);
	CFDictionarySetValue(q, kSecReturnRef, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef item = NULL;
	OSStatus status = SecItemCopyMatching(q, &item);
	CFRelease(q);
	if (status != errSecSuccess) return (int)status;
	SecAccessRef access = NULL;
	status = SecKeychainItemCopyAccess((SecKeychainItemRef)item, &access);
	CFRelease(item);
	if (status != errSecSuccess) return (int)status;
	int open = st_decrypt_open(access);
	CFRelease(access);
	return open;
}

// Whether st_access makes what st_open looks for, for the tests.
static int st_access_is_open(const char *service) {
	SecAccessRef access = NULL;
	if (st_access(service, &access) != errSecSuccess) return 0;
	int open = st_decrypt_open(access);
	CFRelease(access);
	return open;
}

// Keeps the secret, always as a new item with the access above. Updating
// an item in place keeps its old access, so one that is there is deleted
// first, which macOS does without asking. Should the new item not be
// taken with that access, it is added as before, so the key is never
// lost.
static int st_set(const char *service, const char *account, const char *secret, long length) {
	CFMutableDictionaryRef q = st_item(service, account);
	OSStatus status = SecItemDelete(q);
	if (status != errSecSuccess && status != errSecItemNotFound) {
		CFRelease(q);
		return (int)status;
	}
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, length);
	CFDictionarySetValue(q, kSecValueData, data);
	SecAccessRef access = NULL;
	status = st_access(service, &access);
	if (status == errSecSuccess) {
		CFDictionarySetValue(q, kSecAttrAccess, access);
		status = SecItemAdd(q, NULL);
		CFRelease(access);
	}
	if (status != errSecSuccess) {
		CFDictionaryRemoveValue(q, kSecAttrAccess);
		status = SecItemAdd(q, NULL);
	}
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

func (keychain) get() (string, int) {
	secret, status := "", 0
	names(func(service, account *C.char) {
		var out *C.char
		var length C.long
		if status = int(C.st_get(service, account, &out, &length)); status != 0 {
			return
		}
		defer C.st_forget(out, length)
		secret = C.GoStringN(out, C.int(length))
	})
	return secret, status
}

// open is false only when the item is there and kept the old way. When
// it cannot be told, it is left as it is.
func (keychain) open() bool {
	result := C.int(1)
	names(func(service, account *C.char) { result = C.st_open(service, account) })
	return result != 0
}

// accessIsOpen says whether the access a new item gets lets the first
// check pass, without a keychain.
func accessIsOpen() bool {
	service := C.CString(Service)
	defer C.free(unsafe.Pointer(service))
	return C.st_access_is_open(service) == 1
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
