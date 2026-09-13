# CLI Conventions

## Output Streams

Human-readable command results are written to standard output. Diagnostics and errors are written to standard error. Future machine-readable JSON Lines events will be enabled by an explicit output-mode option; they must never be mixed with human-readable standard output.

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | Unexpected internal failure. |
| 2 | Invalid command or command-line usage. |
| 3 | Invalid source input or archive configuration. |
| 4 | Required external dependency is unavailable or incompatible. |
| 5 | Archive, image, or media verification failed. |
| 6 | Operation was cancelled. |

Only codes 0 and 2 are implemented in the Phase 1.1 CLI skeleton. The remaining codes are reserved for later phases.

## Build Identity

`archiver version` reports the build version and, when supplied during the build, the source revision. Build metadata is injected with Makefile variables and is not derived at runtime.
