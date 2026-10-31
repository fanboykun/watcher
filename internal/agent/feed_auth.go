package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Feed credential kinds for HTTP release feeds behind Nexus/Artifactory, an
// authenticating proxy or an object-store gateway.
const (
	FeedAuthBearer = "bearer" // Authorization: Bearer <secret>
	FeedAuthBasic  = "basic"  // Authorization: Basic <username:secret>
	FeedAuthHeader = "header" // <header>: <secret>, e.g. X-JFrog-Art-Api
)

// FeedAuth is an optional credential for an HTTP release feed. It is sent only
// to the feed root's origin, never to other hosts a manifest or redirect names.
type FeedAuth struct {
	Type     string
	Username string // basic user, or sigv4 access key ID
	Header   string // header only
	Region   string // sigv4 only
	Secret   string
}

// NormalizeFeedAuth validates a credential. An empty type means no credential
// and clears every other field.
func NormalizeFeedAuth(auth FeedAuth) (FeedAuth, error) {
	auth.Type = strings.ToLower(strings.TrimSpace(auth.Type))
	auth.Username = strings.TrimSpace(auth.Username)
	auth.Header = strings.TrimSpace(auth.Header)
	auth.Region = strings.TrimSpace(auth.Region)
	auth.Secret = strings.TrimSpace(auth.Secret)
	if auth.Type != FeedAuthSigV4 {
		auth.Region = ""
	}
	switch auth.Type {
	case "", "none":
		return FeedAuth{}, nil
	case FeedAuthBearer:
		auth.Username, auth.Header = "", ""
	case FeedAuthSigV4:
		auth.Header = ""
		if auth.Username == "" {
			return FeedAuth{}, errors.New("S3 feed credentials need an access key ID")
		}
		if auth.Region == "" {
			auth.Region = defaultS3Region
		}
	case FeedAuthBasic:
		auth.Header = ""
		if auth.Username == "" || strings.Contains(auth.Username, ":") {
			return FeedAuth{}, errors.New("basic feed credentials need a username without ':'")
		}
	case FeedAuthHeader:
		auth.Username = ""
		if !validHeaderName(auth.Header) {
			return FeedAuth{}, errors.New("custom feed credentials need a valid header name")
		}
		switch http.CanonicalHeaderKey(auth.Header) {
		case "Host", "Content-Length", "Transfer-Encoding", "Connection":
			return FeedAuth{}, fmt.Errorf("%s cannot carry a feed credential", auth.Header)
		}
	default:
		return FeedAuth{}, fmt.Errorf("feed credential type must be one of: %s, %s, %s, %s", FeedAuthBearer, FeedAuthBasic, FeedAuthHeader, FeedAuthSigV4)
	}
	if auth.Secret == "" || strings.ContainsAny(auth.Secret, "\r\n\x00") {
		return FeedAuth{}, errors.New("feed credential secret is required and must be a single line")
	}
	return auth, nil
}

// validHeaderName reports whether name is an RFC 7230 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func (a FeedAuth) apply(req *http.Request) {
	switch a.Type {
	case FeedAuthBearer:
		req.Header.Set("Authorization", "Bearer "+a.Secret)
	case FeedAuthBasic:
		req.SetBasicAuth(a.Username, a.Secret)
	case FeedAuthHeader:
		req.Header.Set(a.Header, a.Secret)
	case FeedAuthSigV4:
		signSigV4(req, a.Username, a.Secret, a.Region, sigV4Now())
	}
}

// feedAuthTransport adds the credential per request hop. Applying it here,
// rather than on the outgoing request, means redirects and artifact URLs on
// another origin never see it: Go forwards custom headers across redirects.
type feedAuthTransport struct {
	base   http.RoundTripper
	origin *url.URL
	auth   FeedAuth
}

func newFeedAuthTransport(base http.RoundTripper, root string, auth FeedAuth) http.RoundTripper {
	origin, err := url.Parse(root)
	if auth.Type == "" || err != nil {
		return base
	}
	return feedAuthTransport{base: base, origin: origin, auth: auth}
}

func (t feedAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !sameOrigin(req.URL, t.origin) {
		return t.base.RoundTrip(req)
	}
	authed := req.Clone(req.Context())
	t.auth.apply(authed)
	return t.base.RoundTrip(authed)
}

// sameOrigin compares scheme, host and port, so an https feed's credential is
// never sent over plain http even to the same host.
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
