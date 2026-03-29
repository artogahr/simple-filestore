.PHONY: dev css build fmt vet test clean

dev:
	air

css:
	tailwindcss -i ./input.css -o ./static/css/output.css --watch

css-build:
	tailwindcss -i ./input.css -o ./static/css/output.css --minify

build: css-build
	go build -o ./result/simple-filestore ./cmd/server

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -rf ./result
