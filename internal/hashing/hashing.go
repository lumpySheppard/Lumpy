// Package hashing names the checksum formats Lumpy can compute and turns
// each one into a lowercase hex string, the form ASC MHL files use.
package hashing

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"hash"

	"github.com/cespare/xxhash/v2"
	"github.com/zeebo/xxh3"
)

// Format is an ASC MHL hash element name, such as "xxh64".
type Format string

const (
	XXH64  Format = "xxh64"
	XXH3   Format = "xxh3"
	XXH128 Format = "xxh128"
	MD5    Format = "md5"
	SHA1   Format = "sha1"
	SHA256 Format = "sha256"
)

// Preference is the order Lumpy picks a format in when a manifest offers
// several for the same file: the fastest one first.
var Preference = []Format{XXH64, XXH3, XXH128, MD5, SHA1, SHA256}

// Supported reports whether Lumpy can compute f.
func Supported(f Format) bool {
	for _, p := range Preference {
		if p == f {
			return true
		}
	}
	return false
}

// Hasher accumulates bytes and returns the digest as lowercase hex.
type Hasher interface {
	Write(p []byte) (int, error)
	Hex() string
}

type stdHasher struct{ h hash.Hash }

func (s stdHasher) Write(p []byte) (int, error) { return s.h.Write(p) }
func (s stdHasher) Hex() string                 { return hex.EncodeToString(s.h.Sum(nil)) }

type xxh128Hasher struct{ h *xxh3.Hasher }

func (x xxh128Hasher) Write(p []byte) (int, error) { return x.h.Write(p) }
func (x xxh128Hasher) Hex() string {
	b := x.h.Sum128().Bytes()
	return hex.EncodeToString(b[:])
}

// New returns a Hasher for f, or nil if f is not supported.
func New(f Format) Hasher {
	switch f {
	case XXH64:
		return stdHasher{xxhash.New()}
	case XXH3:
		return stdHasher{xxh3.New()}
	case XXH128:
		return xxh128Hasher{xxh3.New()}
	case MD5:
		return stdHasher{md5.New()}
	case SHA1:
		return stdHasher{sha1.New()}
	case SHA256:
		return stdHasher{sha256.New()}
	}
	return nil
}
