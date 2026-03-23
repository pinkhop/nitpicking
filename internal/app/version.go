package app

// version is set at build time via -ldflags:
//
//	go build -ldflags "-X github.com/pinkhop/nitpicking/internal/app.version=1.2.3" -o dist/np ./cmd/np/
//
// During development it defaults to "dev".
var version = "dev"
