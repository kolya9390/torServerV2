package settings

import (
	"sync"
	"testing"
)

func TestSetArgsCopiesInput(t *testing.T) {
	input := &ExecArgs{
		Port:     "8090",
		IP:       "127.0.0.1",
		ProxyURL: "socks5://127.0.0.1:1080",
	}

	SetArgs(input)
	input.Port = "9999"

	got := GetArgs()
	if got == nil {
		t.Fatal("expected args snapshot")
	}

	if got.Port != "8090" {
		t.Fatalf("expected copied port 8090, got %s", got.Port)
	}
}

func TestGetArgsReturnsCopy(t *testing.T) {
	SetArgs(&ExecArgs{Port: "8090"})

	first := GetArgs()
	if first == nil {
		t.Fatal("expected args snapshot")
	}

	first.Port = "7777"

	second := GetArgs()
	if second == nil {
		t.Fatal("expected args snapshot")
	}

	if second.Port != "8090" {
		t.Fatalf("expected immutable snapshot, got %s", second.Port)
	}
}

func TestSetArgsConcurrent(t *testing.T) {
	const workers = 32

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)

		go func(_ int) {
			defer wg.Done()
			SetArgs(&ExecArgs{Port: "80", IP: "127.0.0.1"})

			_ = GetArgs()
		}(i)
	}

	wg.Wait()

	if GetArgs() == nil {
		t.Fatal("expected non-nil args after concurrent set/get")
	}
}
