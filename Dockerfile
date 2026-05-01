# ---- Build Stage ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev nodejs npm

WORKDIR /app

# Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Frontend build
COPY frontend/ frontend/
RUN cd frontend && npm install --silent && npm run build

# Go build (web/TUI mode only — no desktop tags)
COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-w -s" -o dashboard ./cmd/dashboard

# ---- Runtime Stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app
COPY --from=builder /app/dashboard .
RUN mkdir -p stats

EXPOSE 9100

# Default: web dashboard mode with LAN discovery
ENTRYPOINT ["./dashboard"]
CMD ["--web", "--lan"]
