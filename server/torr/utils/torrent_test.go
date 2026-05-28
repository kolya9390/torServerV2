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

func TestMergeDefaultAndRemoteTrackers_PrioritizesStableDefaults(t *testing.T) {
	remote := []string{
		defTrackers[0],
		"udp://remote-a.example.com:6969/announce",
		"UDP://REMOTE-A.EXAMPLE.COM:6969/announce",
		"udp://remote-b.example.com:6969/announce",
		"wss://remote-webtorrent.example.com",
	}

	got := mergeDefaultAndRemoteTrackers(remote)
	nativeDefaults := NormalizeTrackers([][]string{defTrackers}, true, 0)[0]

	if len(got) != len(nativeDefaults)+2 {
		t.Fatalf("len = %d, want %d", len(got), len(nativeDefaults)+2)
	}

	if !reflect.DeepEqual(got[:len(nativeDefaults)], nativeDefaults) {
		t.Fatalf("default trackers are not first")
	}

	wantRemote := []string{
		"udp://remote-a.example.com:6969/announce",
		"udp://remote-b.example.com:6969/announce",
	}
	if !reflect.DeepEqual(got[len(nativeDefaults):], wantRemote) {
		t.Fatalf("remote trackers = %v, want %v", got[len(nativeDefaults):], wantRemote)
	}
}

func TestNormalizeTrackers_DeduplicatesCapsAndPreservesTierOrder(t *testing.T) {
	trackers := [][]string{
		{
			" udp://tracker-a.example.com:6969/announce ",
			"UDP://TRACKER-A.EXAMPLE.COM:6969/announce",
			"wss://webtorrent.example.com",
		},
		{
			"https://tracker-b.example.com/announce",
			"http://tracker-c.example.com/announce",
		},
	}

	got := NormalizeTrackers(trackers, true, 2)
	want := [][]string{
		{"udp://tracker-a.example.com:6969/announce"},
		{"https://tracker-b.example.com/announce"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeTrackers() = %v, want %v", got, want)
	}
}

func TestNormalizeTrackers_FiltersIPv6WhenDisabled(t *testing.T) {
	trackers := [][]string{{
		"udp://[2001:db8::1]:6969/announce",
		"http://tracker.example.com/announce",
	}}

	got := NormalizeTrackers(trackers, false, 0)
	want := [][]string{{"http://tracker.example.com/announce"}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeTrackers() = %v, want %v", got, want)
	}
}

func TestParseNativeTrackerList_DropsWebTorrentTrackers(t *testing.T) {
	raw := `
udp://tracker.example.com:6969/announce
https://tracker.example.com/announce
wss://tracker.example.com/socket
ws://tracker.example.com/socket
`

	want := []string{
		"udp://tracker.example.com:6969/announce",
		"https://tracker.example.com/announce",
	}

	if got := parseNativeTrackerList(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNativeTrackerList() = %v, want %v", got, want)
	}
}
