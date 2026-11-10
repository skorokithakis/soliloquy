# syntax=docker/dockerfile:1

FROM golang:1.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/soliloquy .

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/soliloquy /soliloquy

ENV SOLILOQUY_LISTEN_ADDR=:8080 \
	SOLILOQUY_DB_PATH=/data/soliloquy.db \
	SOLILOQUY_BASE_URL=http://localhost:8080

WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/soliloquy"]
