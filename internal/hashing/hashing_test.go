package hashing

import "testing"

// Expected values come from the Python xxhash package and hashlib.
func TestFormats(t *testing.T) {
	for f, want := range map[Format]string{
		XXH64:  "081de10f4ad5e885",
		XXH3:   "5f338c893f5b2725",
		XXH128: "ab5d26fe85c7c77fc253e0c57dde1679",
		MD5:    "7735f903d119b1e9ed6851878c5e770c",
	} {
		h := New(f)
		h.Write([]byte("lumpy"))
		if got := h.Hex(); got != want {
			t.Errorf("%s: got %s, want %s", f, got, want)
		}
	}
}
