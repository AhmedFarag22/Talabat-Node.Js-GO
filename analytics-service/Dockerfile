# ---------- builder stage: compiles the Go binary ----------
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy module manifests first so `go mod download` is cached when only source changes.
COPY go.mod go.sum ./
RUN go mod download

# Bring in the rest of the source and build a static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/analytics ./cmd/api

# ---------- runtime stage: tiny image ----------
FROM alpine:3.20

ENV APP_ENV=production

WORKDIR /app

# Just the compiled binary — no Go toolchain, no source.
COPY --from=builder /app/bin/analytics ./analytics

# DocumentDB TLS: MONGO_URI references tlsCAFile=global-bundle.pem (relative to /app).
# ADD makes it root-owned/owner-only; chmod so the non-root `nobody` user can read it.
ADD https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem /app/global-bundle.pem
RUN chmod 0644 /app/global-bundle.pem

EXPOSE 4100

# Drop root.
USER nobody

CMD ["./analytics"]
