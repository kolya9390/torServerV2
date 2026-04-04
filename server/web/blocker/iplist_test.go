package blocker

import (
	"net"
	"testing"
)

func TestIPListLookup_Empty(t *testing.T) {
	ipl := New(nil)
	if ipl.NumRanges() != 0 {
		t.Errorf("expected 0 ranges, got %d", ipl.NumRanges())
	}

	r, ok := ipl.Lookup(net.ParseIP("192.168.1.1"))
	if ok {
		t.Error("expected not found for empty list")
	}
	_ = r
}

func TestIPListLookup_SingleIPv4(t *testing.T) {
	ranges := []Range{
		{
			First: net.ParseIP("192.168.1.0"),
			Last:  net.ParseIP("192.168.1.255"),
		},
	}
	ipl := New(ranges)

	tests := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.0", true},
		{"192.168.1.128", true},
		{"192.168.1.255", true},
		{"192.168.1.256", false},
		{"192.168.2.1", false},
		{"10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			_, ok := ipl.Lookup(net.ParseIP(tt.ip))
			if ok != tt.expected {
				t.Errorf("Lookup(%s) = %v, want %v", tt.ip, ok, tt.expected)
			}
		})
	}
}

func TestIPListLookup_IPv6(t *testing.T) {
	ranges := []Range{
		{
			First: net.ParseIP("2001:db8::"),
			Last:  net.ParseIP("2001:db8::ffff"),
		},
	}
	ipl := New(ranges)

	tests := []struct {
		ip       string
		expected bool
	}{
		{"2001:db8::", true},
		{"2001:db8::100", true},
		{"2001:db8::ffff", true},
		{"2001:db9::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			_, ok := ipl.Lookup(net.ParseIP(tt.ip))
			if ok != tt.expected {
				t.Errorf("Lookup(%s) = %v, want %v", tt.ip, ok, tt.expected)
			}
		})
	}
}

func TestIPListLookup_MultipleRanges(t *testing.T) {
	ranges := []Range{
		{First: net.ParseIP("10.0.0.0"), Last: net.ParseIP("10.255.255.255")},
		{First: net.ParseIP("172.16.0.0"), Last: net.ParseIP("172.31.255.255")},
		{First: net.ParseIP("192.168.0.0"), Last: net.ParseIP("192.168.255.255")},
	}
	ipl := New(ranges)

	tests := []struct {
		ip       string
		shouldOk bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"192.168.255.255", true},
		{"8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			_, ok := ipl.Lookup(net.ParseIP(tt.ip))
			if ok != tt.shouldOk {
				t.Errorf("Lookup(%s) = %v, want %v", tt.ip, ok, tt.shouldOk)
			}
		})
	}
}

func TestIPList_LargeList(t *testing.T) {
	ranges := make([]Range, 100)
	for i := 0; i < 100; i++ {
		base := net.ParseIP("192.168.0.0")
		base[2] = byte(i)
		ranges[i] = Range{
			First: base,
			Last:  net.ParseIP("192.168.255.255"),
		}
	}
	ipl := New(ranges)

	if ipl.NumRanges() != 100 {
		t.Errorf("expected 100 ranges, got %d", ipl.NumRanges())
	}

	_, ok := ipl.Lookup(net.ParseIP("192.168.50.100"))
	if !ok {
		t.Error("expected to find IP in range")
	}
}

func TestIPList_BadIP(t *testing.T) {
	ipl := New(nil)

	_, ok := ipl.Lookup(nil)
	if ok {
		t.Error("expected not found for nil IP")
	}
}

func TestRange_Description(t *testing.T) {
	r := Range{
		First:       net.ParseIP("192.168.1.0"),
		Last:        net.ParseIP("192.168.1.255"),
		Description: "LAN",
	}

	expected := "192.168.1.0-192.168.1.255: LAN"
	if r.String() != expected {
		t.Errorf("expected %q, got %q", expected, r.String())
	}
}

func BenchmarkIPListLookup_Found(b *testing.B) {
	ranges := make([]Range, 1000)
	for i := 0; i < 1000; i++ {
		base := net.ParseIP("192.168.1.0")
		base[3] = byte(i % 256)
		ranges[i] = Range{
			First: base,
			Last:  net.ParseIP("192.168.1.255"),
		}
	}
	ipl := New(ranges)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ipl.Lookup(net.ParseIP("192.168.1.128"))
	}
}

func BenchmarkIPListLookup_NotFound(b *testing.B) {
	ranges := make([]Range, 1000)
	for i := 0; i < 1000; i++ {
		base := net.ParseIP("192.168.1.0")
		base[3] = byte(i % 256)
		ranges[i] = Range{
			First: base,
			Last:  net.ParseIP("192.168.1.255"),
		}
	}
	ipl := New(ranges)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ipl.Lookup(net.ParseIP("10.0.0.1"))
	}
}
