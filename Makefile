.PHONY: build install clean test run

VERSION := 0.1.0
LDFLAGS := -ldflags "-s -w -X github.com/contextpilot-dev/memorypilot/cmd.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o memorypilot .

install: build
	cp memorypilot /usr/local/bin/

clean:
	rm -f memorypilot
	rm -rf dist/

test:
	go test -v ./...

run: build
	./memorypilot daemon start

# Cross-compilation
dist: clean
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/memorypilot-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/memorypilot-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/memorypilot-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/memorypilot-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/memorypilot-windows-amd64.exe .

# Check if Ollama is running
check-ollama:
	@curl -s http://localhost:11434/api/tags > /dev/null && echo "✅ Ollama is running" || echo "❌ Ollama not running - start with: ollama serve"

# Pull required Ollama models
setup-ollama: check-ollama
	ollama pull llama3.2
	ollama pull nomic-embed-text
