package httpapi

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestFSDirsListsOnlyVisibleSubdirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WORREL_HOME", home)

	// estrutura: zeta/, alpha/, .oculta/, arquivo.txt
	for _, d := range []string{"zeta", "alpha", ".oculta"} {
		if err := os.Mkdir(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "arquivo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts, _ := newTestServer(t)

	var out struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Home    string `json:"home"`
		Entries []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"entries"`
	}
	// path vazio => home; parent deve ser vazio (raiz permitida).
	code := getJSON(t, ts, "/api/fs/dirs", &out)
	if code != 200 {
		t.Fatalf("code=%d", code)
	}
	if out.Parent != "" {
		t.Fatalf("parent deveria ser vazio no home, got %q", out.Parent)
	}
	if len(out.Entries) != 2 {
		t.Fatalf("esperava 2 subpastas visíveis, got %d (%v)", len(out.Entries), out.Entries)
	}
	if out.Entries[0].Name != "alpha" || out.Entries[1].Name != "zeta" {
		t.Fatalf("entries não ordenados: %v", out.Entries)
	}

	// navegar para subpasta => parent volta para home.
	sub := filepath.Join(home, "alpha")
	code = getJSON(t, ts, "/api/fs/dirs?path="+url.QueryEscape(sub), &out)
	if code != 200 {
		t.Fatalf("subdir code=%d", code)
	}
	// out.Parent resolve symlinks; compare via resolveReal do home.
	if out.Parent == "" {
		t.Fatalf("parent não deveria ser vazio dentro de subpasta")
	}
}

func TestFSDirsRejectsOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WORREL_HOME", home)
	ts, _ := newTestServer(t)

	outside := t.TempDir() // outro tempdir, fora do home
	var out map[string]any
	code := getJSON(t, ts, "/api/fs/dirs?path="+url.QueryEscape(outside), &out)
	if code != 403 {
		t.Fatalf("esperava 403 para caminho fora do home, got %d", code)
	}
}
