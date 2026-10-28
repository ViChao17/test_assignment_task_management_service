// Package docs встраивает OpenAPI-документ, который отдаёт API.
package docs

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
