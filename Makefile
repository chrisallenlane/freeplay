.PHONY: build run fmt vet test check clean docker

build: emulatorjs
	go build -o freeplay .

run: build
	./freeplay -data ./testdata

fmt:
	gofumpt -w .

vet:
	go vet ./...

test:
	go test ./...

check: fmt vet test

clean:
	rm -f freeplay

# Download EmulatorJS for local dev (batch-1/05 creates the script)
emulatorjs:
	@if [ ! -d emulatorjs ]; then ./scripts/download-emulatorjs.sh; fi

docker:
	docker build -t freeplay .
