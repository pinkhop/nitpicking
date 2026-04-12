---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Language Libraries

## Standard Libraries

| Library  | Purpose |
|----------|---------|
| net/http | HTTP and HTTPS servers, server request routing, and clients |
| log/slog | Formatted logging, including JSON logging |

## Open-Source Libraries

This section enumerates common open-source, third-party libraries that are preferred when the standard library does not provide required functionality.

| Name                    | URL                                            | Purpose |
|-------------------------|------------------------------------------------|---------|
| Decimal                 | github.com/shopspring/decimal                  | Arbitrary-precision fixed-point decimal numbers |
| Failsafe-Go             | github.com/failsafe-go/failsafe-go             | Fault tolerance and resilience patterns |
| OpenTelemetry           | go.opentelemetry.io/otel                       | Instrument code with logging, tracing, and metrics to trace activity across distributed systems and measure data about that code's performance and operation |
| pgx                     | github.com/jackc/pgx/v5                        | PostgreSQL database client |
| Rate Limiter            | golang.org/x/time/rate                         | Rate limiter to control how frequently events or operations are allowed to happen |
| urfave/cli              | github.com/urfave/cli/v3                       | Define tool commands and subcommands, parse application flags, build configuration from flags and environment |
| UUID                    | github.com/google/uuid                         | UUID generation and parsing, including UUIDv4 (random), UUIDv5 (SHA1), and UUIDv7 (time-ordered) |
| Watermill               | github.com/ThreeDotsLabs/watermill             | Publish and subscribe to message streams such as Kafka or RabbitMQ; build event-driven applications (EDA); enable event sourcing, CQRS, and sagas; RPC over messages |
| Watermill Kafka Pub/Sub | github.com/ThreeDotsLabs/watermill-kafka/v3    | Connect Watermill to Kafka |
| Watermill Redis Pub/Sub | github.com/ThreeDotsLabs/watermill-redisstream | Connect Watermill to Redis Streams |
| ZombieZen SQLite        | zombiezen.com/go/sqlite                        | Pure-Go SQLite library without database/sql abstractions |
