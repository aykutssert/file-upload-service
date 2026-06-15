FROM golang:1.26.4-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/upload-api \
    ./cmd/upload-api

FROM alpine:3.23

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S app \
    && adduser -S -G app app

COPY --from=build /out/upload-api /usr/local/bin/upload-api

USER app
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/upload-api"]
