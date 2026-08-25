// Package agent implements Watcher's polling, deployment, service-management,
// recovery, and deployment-state runtime.
//
// An Agent supervises one RepoWatcher goroutine per persisted watcher. Each
// RepoWatcher performs a single poll-and-deploy cycle at startup, on its ticker,
// or when the API sends an immediate-check trigger. Watcher changes are applied
// by reconciling the database rows with the currently running goroutines.
//
// Deployment deliberately separates preparation from host mutation. Artifacts
// are downloaded, extracted, validated, and populated with managed release
// configuration before any service is stopped. The Deployer then stops the
// affected services, promotes the staged release, activates the current
// junction, ensures NSSM or IIS registration, starts services, and performs
// health checks.
//
// Failures after services have stopped use compensation that is independent of
// the failed deployment context. A same-version promotion backup restoration is
// distinguished from a rollback to another version, so persisted attempts and
// GitHub Deployment statuses describe what actually happened.
//
// NSSM is the authority for Windows service state. Service lifecycle operations
// accept all seven SCM states, wait through transitional states, resume paused
// services with continue, and verify ambiguous NSSM command results by querying
// status again.
//
// The package is organized by responsibility:
//
//   - agent.go supervises watcher goroutines.
//   - watcher*.go handles polling, configuration, deployment reporting, and
//     failure classification.
//   - deploy.go and deploy_recovery.go orchestrate deploy and compensation.
//   - release_*.go owns release preparation, activation, and retention.
//   - service_manager.go owns NSSM lifecycle state transitions.
//   - service_deployment.go and iis.go own runtime registration.
//   - config_snapshot.go owns managed configuration snapshots.
//   - github_*.go owns GitHub metadata, assets, releases, and deployments.
//   - state.go persists lifecycle state, attempts, logs, and events.
package agent
