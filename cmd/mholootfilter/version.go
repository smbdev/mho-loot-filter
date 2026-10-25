package main

// version is shown in the app. Keep it equal to the versions in winres/winres.json, which version_test.go checks.
const version = "0.9.1"

// testBuild is "T" in test builds, made with go build -ldflags "-X main.testBuild=T"; the app shows it after the
// version. Release builds leave it empty.
var testBuild string
