package push

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	amzDateFormat = "20060102T150405Z"
	sigAlgorithm  = "AWS4-HMAC-SHA256"
)

// signV4 signs req in place with AWS Signature Version 4 for service "s3".
// payloadHash is the hex SHA-256 of the body; it is also sent as
// x-amz-content-sha256, which makes the server verify the bytes it received.
func signV4(req *http.Request, c Credentials, payloadHash string, now time.Time) {
	t := now.UTC()
	amzDate := t.Format(amzDateFormat)
	day := amzDate[:8]
	region := c.Region
	if region == "" {
		region = "auto"
	}

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if c.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", c.SessionToken)
	}

	// Sign host and every x-amz-* / content-* header that is present.
	names := []string{"host"}
	values := map[string]string{"host": req.URL.Host}
	for k, vs := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") || lk == "content-type" || lk == "content-md5" || lk == "range" {
			names = append(names, lk)
			trimmed := make([]string, len(vs))
			for i, v := range vs {
				trimmed[i] = strings.Join(strings.Fields(v), " ")
			}
			values[lk] = strings.Join(trimmed, ",")
		}
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, n := range names {
		canonHeaders.WriteString(n + ":" + values[n] + "\n")
	}
	signed := strings.Join(names, ";")

	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signed,
		payloadHash,
	}, "\n")

	scope := day + "/" + region + "/s3/aws4_request"
	toSign := strings.Join([]string{sigAlgorithm, amzDate, scope, hexSHA256([]byte(canonical))}, "\n")

	key := hmacSHA256([]byte("AWS4"+c.SecretAccessKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))

	req.Header.Set("Authorization", sigAlgorithm+" Credential="+c.AccessKeyID+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string{}, q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode is the SigV4 encoding: unreserved characters stay, everything
// else is %XX of its UTF-8 bytes; "/" stays only when encodeSlash is false.
func uriEncode(s string, encodeSlash bool) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// VerifySigV4 checks a received request's signature against secret. It is
// for test servers (the in-repo fake and cmd/push-devserver): re-signing the
// request as the server saw it catches any canonicalisation drift in transit,
// e.g. the HTTP client re-escaping a Chinese key.
func VerifySigV4(r *http.Request, secret string) bool {
	auth := r.Header.Get("Authorization")
	_, rest, ok := strings.Cut(auth, "Credential=")
	if !ok {
		return false
	}
	cred, rest, _ := strings.Cut(rest, ", SignedHeaders=")
	signed, _, _ := strings.Cut(rest, ", Signature=")
	parts := strings.Split(cred, "/")
	if len(parts) != 5 {
		return false
	}
	at, err := time.Parse(amzDateFormat, r.Header.Get("X-Amz-Date"))
	if err != nil {
		return false
	}
	clone := &http.Request{Method: r.Method, URL: &url.URL{Host: r.Host, Path: r.URL.Path, RawPath: r.URL.RawPath, RawQuery: r.URL.RawQuery}, Header: http.Header{}}
	for _, h := range strings.Split(signed, ";") {
		if h != "host" {
			clone.Header[http.CanonicalHeaderKey(h)] = r.Header.Values(h)
		}
	}
	c := Credentials{Region: parts[2], AccessKeyID: parts[0], SecretAccessKey: secret, SessionToken: r.Header.Get("X-Amz-Security-Token")}
	signV4(clone, c, r.Header.Get("X-Amz-Content-Sha256"), at)
	return hmac.Equal([]byte(clone.Header.Get("Authorization")), []byte(auth))
}
