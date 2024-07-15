generate:
	go generate ./...
lint:
	golangci-lint run --fix
test:
	go test -race -cover ./...
release:
	goreleaser build --snapshot
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o floolishman ${PWD}/cmd/futures/main.go