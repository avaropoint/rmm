package security

import "crypto/sha512"

// hmacSHA512 computes HMAC-SHA-512 without importing crypto/hmac
// to keep the dependency minimal. Uses the standard HMAC construction.
func hmacSHA512(key, message []byte) []byte {
	const blockSize = 128 // SHA-512 block size

	// If key is longer than block size, hash it.
	if len(key) > blockSize {
		h := sha512.Sum512(key)
		key = h[:]
	}

	// Pad key to block size.
	padded := make([]byte, blockSize)
	copy(padded, key)

	ipad := make([]byte, blockSize)
	opad := make([]byte, blockSize)
	for i := range padded {
		ipad[i] = padded[i] ^ 0x36
		opad[i] = padded[i] ^ 0x5c
	}

	// Inner hash: H(ipad || message)
	inner := sha512.New()
	inner.Write(ipad)
	inner.Write(message)
	innerHash := inner.Sum(nil)

	// Outer hash: H(opad || inner_hash)
	outer := sha512.New()
	outer.Write(opad)
	outer.Write(innerHash)

	return outer.Sum(nil)
}

// hmacEqual is a constant-time comparison to prevent timing attacks.
func hmacEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
