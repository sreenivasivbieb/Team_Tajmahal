# ---- Frontend Build Stage ----
FROM node:20-alpine AS frontend-builder

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --ignore-scripts

COPY frontend/ .
RUN npm run build

# ---- Go Build Stage ----
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev git

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum* ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /bin/vyuha-ai ./cmd/server

# ---- Runtime Stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY --from=builder /bin/vyuha-ai .
COPY --from=frontend-builder /src/frontend/dist ./frontend/dist

EXPOSE 8080

ENTRYPOINT ["./vyuha-ai"]
