FROM golang:bookworm AS builder

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=auto

WORKDIR /src
COPY go.mod ./
COPY pkg/ ./pkg/
COPY cmd/ ./cmd/

RUN go build -ldflags="-s -w" -o /out/agy-cockpit ./cmd/agy-cockpit

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    procps \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/agy-cockpit /usr/local/bin/agy-cockpit

EXPOSE 8099

CMD ["/usr/local/bin/agy-cockpit"]
