BINARY := bin/wikichapters
PKG    := ./cmd/wikichapters

.PHONY: all build test fmt vet clean data

all: build

build:
	go build -o $(BINARY) $(PKG)

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

# Rebuild the whole dataset from Wikipedia. Takes several minutes: the API is
# queried serially with a delay, on purpose.
data: build
	$(BINARY) build -out data

clean:
	rm -rf bin
