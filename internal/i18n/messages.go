package i18n

var messages = map[Lang]map[string]string{
	En: {
		"err.non_interactive": "non-interactive: pass --yes",
		"err.no_method":       "no install method for %s",
	},
	PtBR: {
		"err.non_interactive": "modo não interativo: use --yes",
		"err.no_method":       "nenhum método de instalação para %s",
	},
}
