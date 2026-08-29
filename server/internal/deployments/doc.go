// Package deployments owns the deployment lifecycle: the state machine (§25),
// deployment records and history (§26), rollback resolution (§27), and the
// orchestration pipeline (§22).
//
// It drives internal/kamal through the DeploymentEngine interface and never
// touches SSH or Docker directly.
package deployments
