---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Language Libraries

## Standard Libraries

| Library | Purpose |
|---------|---------|
| net/http | HTTP and HTTPS servers, server request routing, and clients |
| log/slog | Formatted logging, including JSON logging |

## Open-Source Libraries

This section enumerates the open-source, third-party libraries that are preferred when the standard library does not provide required functionality.

| Name | URL | Purpose |
|------|-----|---------|
| Decimal | github.com/shopspring/decimal | Arbitrary-precision fixed-point decimal numbers i
| OpenTelemetry | go.opentelemetry.io/otel | Instrument code with logging, tracing, and metrics to trace activity across distributed systems and measure data about that code's performance and operation |
| pgx  | github.com/jackc/pgx/v5 | PostgreSQL database client |
| Rate Limter | golang.org/x/time/rate | Rate limiter to control how frequently events or operations are allowed to happen |
| urfave/cli | github.com/urfave/cli/v3 | Define tool commands and subcommands, parse application flags, build configuration from flags and environment |
| UUID | github.com/google/uuid | UUID generation and parsing, including UUIDv4 and UUIDv5 |
| Watermill | github.com/ThreeDotsLabs/watermill | Publish and subscribe to message streams such as Kafka or RabbitMQ; build event-driven applications (EDA); enable event sourcing, CQRS, and sagas; RPC over messages |
| Watermill Kafka Pub/Sub | github.com/ThreeDotsLabs/watermill-kafka/v3| Connect Watermill to Kafka |
| Watermill Redis Pub/Sub | github.com/ThreeDotsLabs/watermill-redisstream | Connect Watermill to Redis Streams |
