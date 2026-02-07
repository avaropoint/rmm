// Package security provides cryptographic primitives for the platform:
//
//   - TLS certificate generation and management (ECDSA P-384)
//   - Platform identity keypair (Ed25519)
//   - Agent credential signing and verification (HMAC-SHA-512)
//   - Enrollment token and API key generation
//   - HTTP authentication middleware
//
// # Quantum-readiness
//
// Transport layer: Go 1.23+ TLS 1.3 automatically negotiates the
// X25519+ML-KEM-768 hybrid post-quantum key exchange when both peers
// support it — no application code changes required.
//
// Application layer: Agent credentials use HMAC-SHA-512 which is
// quantum-safe for authentication (256-bit security against Grover's
// algorithm). The credential version prefix (v1.) allows a future
// upgrade to ML-DSA (FIPS 204) post-quantum digital signatures once
// available in Go's standard library.
package security
