# Dockerfile
#
# Multi-stage build for a statically-linked Go binary running on a minimal
# distroless image. The build stage compiles the application with CGO disabled
# and pure-Go network/user packages, producing a binary with no system library
# dependencies. The runtime stage copies only the binary into a Chainguard
# static image — no shell, no package manager, minimal attack surface.
#
# Build:
#     docker build -t np .
#     docker build -t np --build-arg VERSION=1.2.3 .
#
# Run:
#     docker run --rm np

################################################################################
# Stage 1: Build
################################################################################

FROM cgr.dev/chainguard/go:latest AS build

# VERSION may be passed at build time to bake a release version into the binary.
# When omitted, the binary defaults to "dev".
ARG VERSION

WORKDIR /src

# Copy dependency manifests first so that module downloads are cached in a
# separate layer. Source changes do not invalidate this layer unless go.mod or
# go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source tree and build the binary. The flags mirror the
# Makefile: CGO off, pure-Go network and user packages, trimmed paths, and
# optional version injection via ldflags.
COPY . .

RUN CGO_ENABLED=0 go build \
    -tags="netgo,osusergo" \
    -trimpath \
    ${VERSION:+-ldflags "-X github.com/pinkhop/nitpicking/internal/app.version=${VERSION}"} \
    -o /out/np \
    ./cmd/np/

################################################################################
# Stage 2: Runtime
################################################################################

FROM cgr.dev/chainguard/static:latest

# Copy the compiled binary from the build stage.
COPY --from=build /out/np /np

ENTRYPOINT ["/np"]
