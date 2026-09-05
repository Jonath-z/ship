package domains

import "testing"

func TestNormalizeHostname(t *testing.T) {
	hostname, violation := normalizeHostname(" API.Example.COM. ")
	if violation != nil || hostname != "api.example.com" {
		t.Fatalf("hostname = %q, violation = %#v", hostname, violation)
	}
	for _, value := range []string{"localhost", "https://example.com", "127.0.0.1", "-api.example.com", "api..example.com"} {
		if _, violation := normalizeHostname(value); violation == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}
