# syntax=docker/dockerfile:1
#
# The wasmtime-go spike on Linux. Build from the repository root:
#   docker build -f bench/function-runtimes/wasmtime-go-spike/Dockerfile --platform linux/arm64 -t fc-wtspike:arm64 .
# Run:  docker run --rm fc-wtspike:arm64 -mode equiv
#
# Built natively per platform (cgo: no cross-compile), on glibc (wasmtime-go
# ships linux-gnu static libraries only).
FROM golang:1.27.4-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY pkg/fcsdk/go.mod pkg/fcsdk/go.sum ./pkg/fcsdk/
COPY bench/function-runtimes/wasmtime-go-spike/go.mod bench/function-runtimes/wasmtime-go-spike/go.sum ./bench/function-runtimes/wasmtime-go-spike/
RUN cd bench/function-runtimes/wasmtime-go-spike && go mod download
COPY . .
ENV GOFLAGS=-buildvcs=false
RUN cd bench/function-runtimes/go-guest && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o /out/bench/go-guest/fcbench-go.wasm . \
 && cd ../wasmtime-go-spike/guests/gospike && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o /out/bench/wasmtime-go-spike/build/gospike.wasm .
WORKDIR /src/bench/function-runtimes/wasmtime-go-spike
# Go 1.27.4 refuses libwasmtime.a as shipped (LLVM bitcode sections, see the
# README). Strip them from a copy inside the module directory (so the check
# still scans it) and build against the copy. The pinned module stays as it is.
RUN set -e; case "$(uname -m)" in aarch64) d=linux-aarch64 ;; x86_64) d=linux-x86_64 ;; esac; \
    M=$(go list -m -f '{{.Dir}}' github.com/bytecodealliance/wasmtime-go/v50); \
    mkdir -p third_party && cp -R "$M" third_party/wasmtime-go && chmod -R u+w third_party/wasmtime-go; \
    ls -l third_party/wasmtime-go/build/$d/libwasmtime.a; \
    objcopy --remove-section=.llvmbc --remove-section=.llvmcmd third_party/wasmtime-go/build/$d/libwasmtime.a; \
    ls -l third_party/wasmtime-go/build/$d/libwasmtime.a; \
    go mod edit -replace github.com/bytecodealliance/wasmtime-go/v50=./third_party/wasmtime-go
RUN go version && go build -tags wasmtime -trimpath -o /out/spike ./cmd/spike \
 && go build -tags wasmtime -trimpath -ldflags='-s -w' -o /out/sizeprobe-wasmtime ./cmd/sizeprobe \
 && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/sizeprobe-wazero ./cmd/sizeprobe \
 && ls -l /out && (ldd /out/sizeprobe-wasmtime || true) && (ldd /out/sizeprobe-wazero || true)
RUN mkdir -p /out/bench/js/dist /out/bench/fixtures \
 && cp ../js/dist/function.js /out/bench/js/dist/ && cp ../fixtures/* /out/bench/fixtures/

FROM debian:bookworm-slim
COPY --from=build /out/bench /bench
COPY --from=build /out/spike /out/sizeprobe-wasmtime /out/sizeprobe-wazero /usr/local/bin/
WORKDIR /bench/wasmtime-go-spike
ENTRYPOINT ["/usr/local/bin/spike"]
