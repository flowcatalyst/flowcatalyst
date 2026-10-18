# Standard Go: exactly what `fcdev fn build|dev|deploy` runs. TinyGo is not supported (docs/function-examples-go-rust.md).
.PHONY: test build clean

test:
	go vet ./... && go test ./...

# Exactly what `fcdev fn build` runs.
build:
	GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o function.wasm .

clean:
	rm -f function.wasm function.h
