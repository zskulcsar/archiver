# Observability

Archiver optionally exports OpenTelemetry traces and metrics over OTLP/HTTP. Telemetry is disabled unless an endpoint is supplied. Export failures never stop archive creation.

## Local Grafana Stack

Start the development-only Grafana LGTM container with Podman:

```sh
make otel-up
```

Grafana is available at <http://127.0.0.1:3000> with `admin` / `admin`. The local OTLP/HTTP endpoint is `http://127.0.0.1:4318`.

Stop the container and retain its named `archiver-otel-lgtm-data` volume:

```sh
make otel-down
```

## Exporting a Run

Pass an endpoint explicitly:

```sh
./bin/archiver create --otel-endpoint http://127.0.0.1:4318 ...
```

Or use the standard environment variable:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 ./bin/archiver create ...
```

Each create operation produces a root `archiver.create` trace with child spans for planning, source-range hashing, PAX tar streaming, GnuPG, PAR2, image creation, verification, and publication. The `archiver.io.bytes` metric records bytes by low-cardinality I/O operation to show read/write-heavy phases without collecting CPU or heap profiles.

Telemetry never includes source paths, logical paths, output paths, archive IDs, passphrase data, hashes, command arguments, or command stderr.

## Grafana Dashboard

Import `docs/grafana/archiver-operations.json` in Grafana using **Dashboards**, **New**, **Import**, and select the bundled Prometheus and Tempo datasources. The dashboard shows archive executions, processed I/O, failed phases, per-operation throughput, phase p95 latency, and recent archive-create traces.
