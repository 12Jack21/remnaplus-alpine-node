package snihealth

import "testing"

func TestIsPublicAddressRejectsPrivateReservedAndDocumentationRanges(t *testing.T) {
	t.Parallel()

	blocked := []string{
		"0.0.0.0", "10.0.0.1", "100.64.0.1", "127.0.0.1", "169.254.1.1",
		"172.16.0.1", "192.0.2.1", "192.168.1.1", "198.18.0.1", "198.51.100.1",
		"203.0.113.1", "224.0.0.1", "255.255.255.255", "::", "::1",
		"::ffff:192.0.2.1", "100::1", "2001:db8::1", "fc00::1", "fe80::1", "ff02::1",
		"not-an-address",
	}
	for _, address := range blocked {
		if IsPublicAddress(address) {
			t.Fatalf("%s must be rejected", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if !IsPublicAddress(address) {
			t.Fatalf("%s must be accepted", address)
		}
	}
}
