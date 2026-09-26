package calculatorresult

import "embed"

// FS exposes shared robot templates to packages outside this directory.
//
//go:embed calculator_result.html logo.png
var FS embed.FS
