// Package e2e holds end-to-end tests only. They build the real assessgap
// binary and drive it the way people do: commands and their exit codes on
// the frozen fixture, and "serve" on a random port with a walk of every
// route. Unit tests call cli.Run and the web handler in-process; these catch
// what only the built program shows (flag wiring, embedded files, process
// exit codes, startup and shutdown). Run them with go test ./internal/e2e;
// scripts/check.sh includes them.
package e2e
