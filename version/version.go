package version

// version is overwritten at compile time by passing
// -ldflags -X code.cloudfoundry.org/korifi/version.Version=<version>
var Version = "v9999.99.99-local.dev"
