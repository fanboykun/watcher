package agent

import "testing"

func TestParseServiceState(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "running", input: "SERVICE_RUNNING", expect: "SERVICE_RUNNING"},
		{name: "stopped with text", input: "AppSvc: SERVICE_STOPPED", expect: "SERVICE_STOPPED"},
		{name: "start pending lowercase", input: "service_start_pending", expect: "SERVICE_START_PENDING"},
		{name: "unknown", input: "weird output", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseServiceState(tt.input)
			if got != tt.expect {
				t.Fatalf("parseServiceState(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestIsServiceMissingOutput(t *testing.T) {
	if !isServiceMissingOutput("OpenService(): The specified service does not exist as an installed service.") {
		t.Fatalf("expected missing-service output to be detected")
	}
	if isServiceMissingOutput("SERVICE_RUNNING") {
		t.Fatalf("running output must not be treated as missing")
	}
}
