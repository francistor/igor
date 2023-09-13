package resources

import (
	"embed"
)

// Config objects from resources directory may be retrieved using resource://<whatever>

//go:embed *
var Fs embed.FS
