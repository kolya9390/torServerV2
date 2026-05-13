package utils

import (
	"reflect"
	"testing"
)

func TestParseTrackerList(t *testing.T) {
	raw := `
udp://1.2.3.4:6969/announce

http://tracker.example.com:80/announce
https://tracker.example.com/announce
wss://tracker.example.com/socket
ws://tracker.example.com/socket
garbage
ftp://tracker.example.com
`

	want := []string{
		"udp://1.2.3.4:6969/announce",
		"http://tracker.example.com:80/announce",
		"https://tracker.example.com/announce",
		"wss://tracker.example.com/socket",
		"ws://tracker.example.com/socket",
	}

	if got := parseTrackerList(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTrackerList() = %v, want %v", got, want)
	}
}
