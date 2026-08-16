# syntax=docker/dockerfile:1
FROM golang:1.26.5 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# The SQLite driver is pure Go, so the binary can be static and the final image distroless.
RUN CGO_ENABLED=0 go build -o /out/wattfeder ./cmd/wattfeder

# distroless has no shell to mkdir/chown in, so /data is prepared here and copied over owned by
# the nonroot user the final image runs as.
RUN mkdir -p /data && chown 65532:65532 /data

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/wattfeder /wattfeder
COPY --from=builder --chown=65532:65532 /data /data
USER nonroot:nonroot
VOLUME ["/data"]
ENTRYPOINT ["/wattfeder"]
CMD ["-database", "/data/wattfeder.db"]
