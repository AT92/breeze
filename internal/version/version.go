package version

// Version is the application version. It can be overridden at build time via:
//
//	go build -ldflags "-X breeze/internal/version.Version=1.2.3"
var Version = "0.1.0"
