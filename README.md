# xmtpd

XMTP node implementation.

[![Test](https://github.com/xmtp/xmtpd/actions/workflows/test.yml/badge.svg)](https://github.com/xmtp/xmtpd/actions/workflows/test.yml)
[![Build](https://github.com/xmtp/xmtpd/actions/workflows/build.yml/badge.svg)](https://github.com/xmtp/xmtpd/actions/workflows/build.yml)
[![Publish](https://github.com/xmtp/xmtpd/actions/workflows/publish.yml/badge.svg)](https://github.com/xmtp/xmtpd/actions/workflows/publish.yml)

## Development

Build and install dependencies:

```sh
dev/up
```

Start a node:

```sh
dev/start
```

Run tests:

```sh
dev/test
```

#### Monitoring

Visit local [Prometheus](https://prometheus.io/) UI to explore metrics:

```sh
open http://localhost:9090
```

Visit local [Jaeger](https://www.jaegertracing.io/) UI to explore traces:

```sh
open http://localhost:16686
```

## Devnet

See [dev/net/README.md](./dev/net/README.md) for instructions on creating clusters of XMTP nodes locally or on cloud platforms like AWS and GCP.

## Resources

* OpenTelemetry intro <https://www.komu.engineer/blogs/11/opentelemetry-and-go>
