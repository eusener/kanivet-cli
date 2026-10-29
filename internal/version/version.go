// Package version extrai e compara versões numéricas ("1.2.3").
package version

import (
	"regexp"
	"strconv"
	"strings"
)

// Extract devolve o primeiro grupo de captura de regex em output.
func Extract(output, regex string) (string, bool) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(output)
	if len(m) < 2 || m[1] == "" {
		return "", false
	}
	return m[1], true
}

// Compare compara parte a parte; partes ausentes valem 0 e sufixos ("-rc1") são ignorados.
func Compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}
