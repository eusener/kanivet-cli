package i18n

var messages = map[Lang]map[string]string{
	En: {
		"err.non_interactive": "non-interactive: pass --yes",
		"err.no_method":       "no install method for %s",
		"reason.platform":     "not available on %s",
		"reason.arch":         "not available on %s",
		"reason.graphical":    "no graphical session",
		"reason.macos":        "requires macOS %s or newer",
		"reason.systemd":      "requires systemd",
		"reason.distro":       "requires one of: %s",
	},
	PtBR: {
		"err.non_interactive": "modo não interativo: use --yes",
		"err.no_method":       "nenhum método de instalação para %s",
		"reason.platform":     "indisponível em %s",
		"reason.arch":         "indisponível em %s",
		"reason.graphical":    "sem sessão gráfica",
		"reason.macos":        "requer macOS %s ou mais novo",
		"reason.systemd":      "requer systemd",
		"reason.distro":       "requer uma destas: %s",
	},
}
