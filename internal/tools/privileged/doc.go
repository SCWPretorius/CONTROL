// Package privileged contains the local privileged tool layer for filesystem
// writes and shell execution. The package is intentionally scoped to app-owned
// local tooling so later runtime wiring can expose it without coupling it to
// Google integrations.
package privileged
