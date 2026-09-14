// //go:build debug

package main

import (
	"encoding/json"
)

// Unused: nothing calls PrettyPrint. Note also that the build tag on line 1 is
// commented out, so this file compiles into every build rather than only the
// debug ones. Left exactly as found - flagging it, not touching it.
func PrettyPrint(data any) string {
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}
