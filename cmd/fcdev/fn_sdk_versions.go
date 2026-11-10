package main

// The function SDK releases `fcdev fn init` points new projects at. Each is the content of the
// SDK's VERSION file under clients/ as of this fcdev release; TestFnSDKVersionsMatchVersionFiles
// fails when one drifts, so a template can never name a release that was not published.
const (
	// fnGoSDKVersion: Go module github.com/flowcatalyst/flowcatalyst/clients/fn-go, tag
	// clients/fn-go/v<version> (clients/fn-go/VERSION).
	fnGoSDKVersion = "0.4.0"
	// fnTSSDKVersion: @flowcatalyst/fn from github:flowcatalyst/fn-ts-sdk, tag v<version>
	// (clients/fn-ts/VERSION).
	fnTSSDKVersion = "0.6.0"
)

// fnRustSDKDep is the Cargo dependency for flowcatalyst-fn. The crate is not on crates.io and has
// no split repository: cargo finds it inside the platform repository, at the release tag of the
// fcdev that wrote the project.
func fnRustSDKDep() string {
	return `{ git = "https://github.com/flowcatalyst/flowcatalyst", tag = "fcdev/v` + version() + `" }`
}

// fnInitData is what the fn init templates render against.
type fnInitData struct {
	Name, App, SDK string
	GoVer, TSVer   string
	RustDep        string
}

func newFnInitData(name, app, sdk string) fnInitData {
	return fnInitData{Name: name, App: app, SDK: sdk, GoVer: fnGoSDKVersion, TSVer: fnTSSDKVersion, RustDep: fnRustSDKDep()}
}
