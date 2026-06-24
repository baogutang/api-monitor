FROM node:22-alpine AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
ARG GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
ARG VERSION=0.1.0-dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X api-monitor/internal/version.Version=${VERSION} -X api-monitor/internal/version.Commit=${COMMIT} -X api-monitor/internal/version.Date=${DATE}" \
    -o /out/api-monitor ./cmd/api-monitor

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/api-monitor /usr/local/bin/api-monitor
COPY migrations ./migrations
COPY --from=web-build /src/web/dist ./web/dist
USER app
EXPOSE 8080
ENTRYPOINT ["api-monitor"]
CMD ["api"]
