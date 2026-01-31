.PHONY: dev build templ clean install run

install:
	@echo "Installing dependencies..."
	go mod download
	go install github.com/a-h/templ/cmd/templ@latest
	@echo "Done!"

templ:
	@echo "Generating templ files..."
	templ generate

dev:
	@echo "Starting development server..."
	templ generate --watch --proxy="http://localhost:8080" --cmd="go run cmd/server/main.go"

build: templ
	@echo "Building production binary..."
	go build -ldflags="-s -w" -o ./bin/server.exe cmd/server/main.go
	@echo "Binary created at ./bin/server.exe"

clean:
	@echo "Cleaning up..."
	rm -rf ./bin tmp/
	find . -name "*_templ.go" -delete
	@echo "Clean complete!"

run: templ
	go run cmd/server/main.go
