# syntax=docker/dockerfile:1

# Build for the target platform rather than emulating it under QEMU. The build
# is pure Go (CGO is off), so the compiler can run natively on the builder and
# emit binaries for each target architecture itself, which makes the arm64
# half of the multi-arch image build as fast as the amd64 one.
FROM --platform=$BUILDPLATFORM golang:1.24 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
	go build -trimpath -ldflags="-s -w" -o /out/soliloquy .

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
