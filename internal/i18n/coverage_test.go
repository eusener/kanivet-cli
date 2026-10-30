package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAllUsedKeysExist garante que toda chave i18n.T("...") usada no código tem tradução.
func TestAllUsedKeysExist(t *testing.T) {
	re := regexp.MustCompile(`i18n\.T\("([a-z_]+\.[a-z_.]+)"`)
	reN := regexp.MustCompile(`i18n\.N\("([a-z_]+\.[a-z_.]+)"`)
	_ = filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, _ := os.ReadFile(path)
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			if _, ok := messages[En][m[1]]; !ok {
				t.Errorf("%s: key %q has no en message", path, m[1])
			}
		}
		// mensagens de contagem: plural (key) e singular (key.one) nos dois idiomas
		for _, m := range reN.FindAllStringSubmatch(string(data), -1) {
			for _, k := range []string{m[1], m[1] + ".one"} {
				for _, l := range []Lang{En, PtBR} {
					if _, ok := messages[l][k]; !ok {
						t.Errorf("%s: key %q has no %s message", path, k, l)
					}
				}
			}
		}
		return nil
	})
}
