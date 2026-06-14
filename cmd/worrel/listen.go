package main

import (
	"fmt"
	"net"
	"strconv"
)

// listenWithFallback escuta em addr; se a porta estiver ocupada, tenta as
// próximas (até maxTries portas). Retorna o listener efetivo. Porta 0 (efêmera)
// é tentada uma única vez (o SO escolhe uma livre).
func listenWithFallback(addr string, maxTries int) (net.Listener, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		return net.Listen("tcp", addr)
	}
	var lastErr error
	for i := 0; i < maxTries; i++ {
		try := net.JoinHostPort(host, strconv.Itoa(port+i))
		ln, err := net.Listen("tcp", try)
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("nenhuma porta livre a partir de %s: %w", addr, lastErr)
}
