// Package catalogdata embute o catálogo declarativo no binário.
package catalogdata

import "embed"

//go:embed catalog.yaml all:categories all:items all:profiles all:configs
var FS embed.FS
