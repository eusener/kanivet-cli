// Package i18n traduz os textos da interface (inglês padrão, pt-BR opcional).
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

type Lang string

const (
	En   Lang = "en"
	PtBR Lang = "pt-BR"
)

var current atomic.Value

func init() { current.Store(En) }

// DetectLang usa LC_ALL e, na falta dele, LANG; prefixo "pt" escolhe pt-BR.
func DetectLang(env map[string]string) Lang {
	v := env["LC_ALL"]
	if v == "" {
		v = env["LANG"]
	}
	if strings.HasPrefix(strings.ToLower(v), "pt") {
		return PtBR
	}
	return En
}

func SetLang(l Lang) { current.Store(l) }

func Current() Lang { return current.Load().(Lang) }

// T traduz key no idioma atual; uma chave ausente volta como está.
func T(key string, args ...any) string {
	msg, ok := messages[Current()][key]
	if !ok {
		msg, ok = messages[En][key]
	}
	if !ok {
		msg = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// N traduz uma mensagem de contagem: com n == 1 usa a forma do singular (key+".one"),
// quando existe; senão, a do plural (key). O n é o argumento da mensagem.
func N(key string, n int) string {
	if n == 1 {
		one := key + ".one"
		if _, ok := messages[Current()][one]; ok {
			return T(one, n)
		}
		if _, ok := messages[En][one]; ok {
			return T(one, n)
		}
	}
	return T(key, n)
}

// Text é um texto do catálogo: string simples (inglês) ou mapa por idioma.
type Text map[Lang]string

func (t *Text) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*t = Text{En: n.Value}
		return nil
	}
	m := map[Lang]string{}
	if err := n.Decode(&m); err != nil {
		return err
	}
	*t = m
	return nil
}

// Get devolve o texto no idioma atual, com fallback para inglês.
func (t Text) Get() string {
	if s, ok := t[Current()]; ok {
		return s
	}
	return t[En]
}
