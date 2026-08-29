// Package monitoring collects server metrics, container metrics, and
// application health (spec §29), and compares desired state against observed
// actual state to report infrastructure drift (§55).
//
// V1 detects and reports drift. It never auto-remediates.
package monitoring
