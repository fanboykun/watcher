package agent

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestSignSigV4MatchesAWSReference reproduces the "GET Object" example from
// the Amazon S3 Signature Version 4 documentation.
func TestSignSigV4MatchesAWSReference(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-9")
	signSigV4(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

func TestAWSURIEncode(t *testing.T) {
	if got := awsURIEncode("/releases/services/prs/v0.1.0+build/app (1).zip", false); got != "/releases/services/prs/v0.1.0%2Bbuild/app%20%281%29.zip" {
		t.Fatalf("path encoding = %q", got)
	}
	if got := awsURIEncode("a/b c", true); !strings.EqualFold(got, "a%2Fb%20c") {
		t.Fatalf("query encoding = %q", got)
	}
}
