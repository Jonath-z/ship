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

func TestDomainDNSGuidanceUsesOnlyIPv4ARecords(t *testing.T) {
	records := requiredDNSRecords("api.example.com", []string{"203.0.113.10", "2001:db8::10"})
	if len(records) != 1 || records[0].Type != "A" || records[0].Value != "203.0.113.10" {
		t.Fatalf("records = %#v", records)
	}
}
