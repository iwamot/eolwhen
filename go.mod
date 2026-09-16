module github.com/iwamot/eolwhen

// The oldest Go release upstream still supports, which is also the oldest
// entry in the compatibility matrix (.github/workflows/compatibility.yml).
// This directive is what promises `go install` works there, so the two move
// together when a release leaves support. Raising it on its own would drop
// that promise, and — since GOTOOLCHAIN defaults to auto — leave the older
// matrix entry downloading the newer toolchain and testing it twice.
go 1.26

require github.com/goccy/go-yaml v1.19.2

require github.com/BurntSushi/toml v1.6.0
