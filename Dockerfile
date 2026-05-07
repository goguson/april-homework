FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=v0.0.0
ARG COMMIT_HASH=local
ARG BUILD_DATE=local
RUN go build -trimpath -ldflags="-s -w -X github.com/goguson/homework-april-1/internal/config.Version=${VERSION} -X github.com/goguson/homework-april-1/internal/config.CommitHash=${COMMIT_HASH} -X github.com/goguson/homework-april-1/internal/config.BuildDate=${BUILD_DATE}" -o /out/rates-service ./cmd/rates-service

FROM alpine:3.22
RUN adduser -D -H app
USER app
COPY --from=builder /out/rates-service /usr/local/bin/rates-service
EXPOSE 8888
ENTRYPOINT ["rates-service"]
CMD ["serve"]

