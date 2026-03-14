// Package freeplay provides embedded assets for the freeplay server.
package freeplay

import "embed"

// FrontendFS contains the embedded frontend assets.
//
//go:embed frontend
var FrontendFS embed.FS

// EmulatorjsFS contains the embedded EmulatorJS assets.
//
//go:embed emulatorjs
var EmulatorjsFS embed.FS
