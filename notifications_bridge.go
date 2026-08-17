//go:build linux

package main

/*
#include <stdlib.h>
extern void shell_notif_close_impl(unsigned long id);
*/
import "C"

// closeWebNotification closes a tab's WebKit notification on the GTK main
// thread (called when the corresponding desktop notification is dismissed).
// The C shim (tabs_shell.go) marshals the call onto the GTK main thread so it
// is safe to invoke from any goroutine.
func closeWebNotification(id uint64) {
	C.shell_notif_close_impl(C.ulong(id))
}
