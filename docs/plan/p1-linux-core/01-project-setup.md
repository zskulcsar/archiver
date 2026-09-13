# Phase 1.1: Project Setup Plan

## Objective

Create the monorepo foundation for a Go CLI that can grow into Linux, Windows, macOS, native GUI, and packaging deliverables without reorganizing the portable core later.

This phase creates buildable project structure and development quality gates. It does not create archive behavior.

## Inputs

- [Cross-platform CLI high-level plan](../cli-imp-high-level.md)
- [Archive workflow and tooling specification](../../bd-xl-multi-disc-archive-workflow-and-tooling-spec.md)

## Completion Criteria

- The repository has one root Go module and produces an `archiver` CLI binary.
- The default build, test, formatting, static analysis, and documentation checks run through Makefile targets.
- The source layout prevents portable business logic from depending on operating-system integrations.
- The repository reserves locations for future GUIs and packaging without implementing either.
- CI verifies the Linux phase-one build and tests. Cross-compilation scaffolding is present but Windows/macOS release artifacts are not required in this phase.

## Intended Repository Layout

```text
cmd/archiver/                 CLI composition root
internal/
  app/                        CLI use cases and orchestration
  domain/                     Portable archive, manifest, plan, and artifact types
  adapters/                   External-tool and filesystem adapter contracts
  platform/linux/             Linux-only adapter implementations, added in Phase 1.3
apps/
  gui/linux/                  Reserved for a future native Linux GUI
  gui/windows/                Reserved for a future native Windows GUI
  gui/macos/                  Reserved for a future native macOS GUI
packaging/
  linux/                      Reserved for Linux packages and release assets
  windows/                    Reserved for Windows packages and release assets
  macos/                      Reserved for macOS packages and release assets
scripts/                      Repeatable development and release helpers
docs/
  plan/                       Product and implementation plans
```

Create empty future directories only when they need a tracked marker or documentation file. Do not introduce GUI frameworks, package-manager definitions, or release automation in this phase.

## Work

### [ ] 1. Bootstrap the Go Module

1. Select the canonical module path before the first release artifact is published.
2. Add `go.mod` at the repository root using the supported Go version selected for the project.
3. Add `cmd/archiver/main.go` as the composition root with a minimal version/help command.
4. Keep the command layer thin: it parses input, wires dependencies, invokes an application use case, renders events, and maps errors to exit codes.

### [ ] 2. Establish Dependency Direction

1. Define portable domain types under `internal/domain`.
2. Define use cases and adapter interfaces under `internal/app` and `internal/adapters`.
3. Keep process execution, filesystem details, and OS-specific code outside the domain.
4. Reserve `internal/platform/linux` for the Linux implementations introduced in Phase 1.3.
5. Do not add Windows/macOS build tags or adapters until their platform plans are implemented.

### [ ] 3. Establish CLI and Event Conventions

1. Reserve a single executable name, `archiver`, on every platform.
2. Define conventions for human-readable standard output, diagnostic standard error, and JSON Lines machine events.
3. Reserve documented exit-code ranges for invalid input, unavailable dependencies, verification failure, cancellation, and unexpected failures.
4. Include the build version and source revision in the CLI version command and generated reports where available.

### [ ] 4. Establish Quality Tooling

1. Add Makefile targets for formatting, unit tests, race tests where concurrency is introduced, static analysis, and an aggregate verification target.
2. Use `gofmt`, `go vet`, and the repository's selected linter configuration.
3. Configure CI to run the same aggregate verification target on Linux for pull requests.
4. Configure dependency updates and release automation only after the first buildable CLI exists and their required credentials/policies are agreed.

### [ ] 5. Establish Repository Policies

1. Update `.gitignore` for Go build outputs, temporary archives/images, test fixtures generated at runtime, and local tool configuration without ignoring source fixtures or documentation.
2. Add contributor-facing documentation covering required Go version, supported local commands, and the fact that optical tooling is optional until Phase 1.3.
3. Define a fixture policy: small synthetic files are committed; large images and real archives are generated during tests and never committed.
4. Define a policy for external-tool binaries: phase-one development discovers system-installed tools; it does not download or silently execute unverified tools.

## Verification

1. Run the aggregate Makefile verification target on a clean Linux checkout.
2. Build the CLI with no optional optical tools installed and confirm that `archiver --help` and `archiver version` work.
3. Confirm the source tree can cross-compile the portable CLI for Windows and macOS without importing Linux-only packages into the portable core.
4. Confirm all future GUI and packaging paths are documentation-only or tracked placeholders, with no framework or packaging dependency added.

## Explicitly Deferred

- Archive planning and manifest parsing.
- Encryption, parity, image generation, and verification.
- Linux external-tool integration and direct optical writing.
- Windows and macOS integrations.
- GUI applications, installer formats, signing, and release publishing.
