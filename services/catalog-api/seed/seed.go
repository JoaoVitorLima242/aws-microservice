// Package seed embeds the product seed file into the binary, so the service
// does not depend on the file being present at runtime.
package seed

import _ "embed"

//go:embed products.json
var Products []byte
