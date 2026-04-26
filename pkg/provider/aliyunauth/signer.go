// Package aliyunauth implements the Aliyun (Alibaba Cloud) RPC-style API
// request signing shared by the dns/aliyun and registrar/aliyun providers.
//
// Signing algorithm (per Aliyun RPC signature spec):
//  1. Sort all parameters by key name lexicographically.
//  2. URL-encode each key and value using RFC 3986 encoding (percent-encode
//     everything except A-Z a-z 0-9 - _ . ~).
//  3. Build a canonical query string: "k1=v1&k2=v2&..."
//  4. Build the string-to-sign: "GET&%2F&" + PercentEncode(canonicalQS)
//  5. HMAC-SHA1 with signing key = AccessKeySecret + "&"
//  6. Base64-encode the digest and append as the Signature parameter.
package aliyunauth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Signer holds Aliyun API credentials and produces signed request URLs.
type Signer struct {
	AccessKeyID     string
	AccessKeySecret string
}

// New creates a new Signer from the given credentials.
func New(accessKeyID, accessKeySecret string) *Signer {
	return &Signer{
		AccessKeyID:     accessKeyID,
		AccessKeySecret: accessKeySecret,
	}
}

// CommonParams returns the set of parameters that every Aliyun RPC request
// must include. The Timestamp and SignatureNonce values are freshly generated
// on each call. Callers must merge these with their action-specific params
// before passing to SignedURL.
func (s *Signer) CommonParams(version string) map[string]string {
	return map[string]string{
		"Format":           "JSON",
		"Version":          version,
		"AccessKeyId":      s.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   uuid.New().String(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// SignedURL builds a fully-signed GET URL for the given base URL and merged
// parameter map. The map is consumed without modification; callers should pass
// the result of merging CommonParams and their action params.
func (s *Signer) SignedURL(baseURL string, params map[string]string) string {
	// Step 1: sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Step 2: build canonical query string
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, Encode(k)+"="+Encode(params[k]))
	}
	canonicalQS := strings.Join(parts, "&")

	// Step 3: build string-to-sign
	stringToSign := "GET&%2F&" + Encode(canonicalQS)

	// Step 4: HMAC-SHA1, key = AccessKeySecret + "&"
	signingKey := s.AccessKeySecret + "&"
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Step 5: append Signature and build final URL
	return baseURL + "/?" + canonicalQS + "&Signature=" + Encode(sig)
}

// Encode percent-encodes a string per RFC 3986, leaving unreserved characters
// (A-Z a-z 0-9 - _ . ~) unencoded. Required by Aliyun's signing specification.
func Encode(s string) string {
	var buf strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
			(b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~' {
			buf.WriteByte(b)
		} else {
			fmt.Fprintf(&buf, "%%%02X", b)
		}
	}
	return buf.String()
}
