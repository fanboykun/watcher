package agent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// FeedAuthSigV4 signs requests with AWS Signature Version 4, for feeds in a
// private S3-compatible bucket (AWS S3, MinIO, Cloudflare R2). The feed URL
// already names the bucket (path-style or virtual-hosted), so signing the
// request as sent needs no separate bucket or addressing setting.
const FeedAuthSigV4 = "sigv4"

// defaultS3Region is what MinIO and most S3-compatible stores accept when no
// region is configured.
const defaultS3Region = "us-east-1"

// emptyPayloadSHA256 is the payload hash of the bodiless GET requests feeds make.
const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sigV4Now is the signing clock; tests pin it to reproduce reference signatures.
var sigV4Now = time.Now

// signSigV4 signs a bodiless S3 request in place. It signs host, range and
// x-amz-* headers, and sets the request path to exactly the signed encoding.
func signSigV4(req *http.Request, accessKey, secretKey, region string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	date := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", emptyPayloadSHA256)

	canonicalURI := awsURIEncode(req.URL.Path, false)
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	req.URL.RawPath = canonicalURI

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	headers := map[string]string{"host": host}
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "range" || strings.HasPrefix(lower, "x-amz-") {
			headers[lower] = strings.TrimSpace(strings.Join(values, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalRequest := strings.Join([]string{
		req.Method, canonicalURI, canonicalQuery(req.URL.Query()), canonicalHeaders.String(), signedHeaders, emptyPayloadSHA256,
	}, "\n")
	scope := date + "/" + region + "/s3/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), date)
	for _, part := range []string{region, "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", accessKey, scope, signedHeaders, signature))
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// canonicalQuery encodes query parameters sorted by key, then value.
func canonicalQuery(values url.Values) string {
	pairs := make([]string, 0, len(values))
	for key, vals := range values {
		for _, val := range vals {
			pairs = append(pairs, awsURIEncode(key, true)+"="+awsURIEncode(val, true))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// awsURIEncode percent-encodes everything except RFC 3986 unreserved
// characters (and "/" in paths), as Signature V4 requires.
func awsURIEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
