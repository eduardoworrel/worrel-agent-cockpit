package main

import (
	"os/exec"
	"runtime"
)

// browserCommand devolve o comando do SO para abrir uma URL no navegador padrão.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default: // linux, *bsd, etc.
		return "xdg-open", []string{url}
	}
}

// openBrowser abre url no navegador padrão; erros são silenciosos (best-effort).
func openBrowser(url string) {
	name, args := browserCommand(runtime.GOOS, url)
	_ = exec.Command(name, args...).Start()
}
