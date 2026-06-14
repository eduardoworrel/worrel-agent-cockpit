package main

import (
	"net"
	"testing"
)

func TestListenWithFallbackPicksNextPort(t *testing.T) {
	// ocupa uma porta efêmera
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	addr := first.Addr().String()

	// tentar escutar no MESMO addr deve cair na próxima porta livre
	ln, err := listenWithFallback(addr, 5)
	if err != nil {
		t.Fatalf("esperava fallback, deu erro: %v", err)
	}
	defer ln.Close()

	_, p1, _ := net.SplitHostPort(addr)
	_, p2, _ := net.SplitHostPort(ln.Addr().String())
	if p1 == p2 {
		t.Fatalf("porta não mudou no fallback: %s", p2)
	}
}

func TestListenWithFallbackBindsRequested(t *testing.T) {
	// porta livre (0 = efêmera) deve ser usada direto
	ln, err := listenWithFallback("127.0.0.1:0", 5)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	defer ln.Close()
}
