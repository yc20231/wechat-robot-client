package safetyreminder

import "embed"

// Assets contains the reviewed poster background, layout, and default topic bank.
//
//go:embed poster.html topics.json assets/poster-background.png
var Assets embed.FS
