generate:
	go generate ./...
lint:
	golangci-lint run --fix
test:
	go test -race -cover ./...
release:
	goreleaser build --snapshot
build-mac:
	CGO_ENABLED=0 go build -o floolishman-mac ${PWD}/cmd/futures/main.go
build-main:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o floolishman ${PWD}/cmd/futures/main.go
build-guider:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o floolishman-guider ${PWD}/cmd/guider/main.go