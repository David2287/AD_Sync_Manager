# ── Stage 1: Build frontend ────────────────────────────────────────────────────
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --prefer-offline

COPY frontend/ ./
RUN npm run build
# Output lands in /app/web/dist (configured in vite.config.ts outDir: ../web/dist)

# ── Stage 2: Build Go binary ───────────────────────────────────────────────────
FROM golang:1.21-alpine AS go-builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Copy the compiled frontend into web/dist so //go:embed picks it up.
COPY --from=frontend-builder /app/web/dist ./web/dist

# CGO disabled — we use modernc.org/sqlite (pure Go), no C toolchain needed.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ad-sync-manager ./cmd/server

# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=go-builder /ad-sync-manager /app/ad-sync-manager

RUN mkdir -p /data/notes && chmod 750 /data/notes

EXPOSE 8080

RUN addgroup -S adsync && adduser -S adsync -G adsync
USER adsync

ENTRYPOINT ["/app/ad-sync-manager"]
