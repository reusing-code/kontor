FROM node:22-alpine AS frontend
WORKDIR /app
RUN npm install -g bun
COPY frontend/package.json frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY frontend/ .
COPY contract-import-spec.txt ../
RUN bun run build

FROM golang:1.26-alpine AS backend
WORKDIR /app
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build \
    -ldflags "-X github.com/reusing-code/kontor/backend/internal/version.Version=${VERSION} -X github.com/reusing-code/kontor/backend/internal/version.Commit=${COMMIT} -X github.com/reusing-code/kontor/backend/internal/version.BuildDate=${BUILD_DATE}" \
    -o server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=backend /app/server .
COPY --from=frontend /app/dist ./static
ENV STATIC_DIR=/app/static
ENV DB_PATH=/app/data
ENV LOG_FORMAT=json
ENV ENVIRONMENT=production
EXPOSE 8080
ENTRYPOINT ["./server"]
