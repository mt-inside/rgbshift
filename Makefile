.DEFAULT_GOAL := run

.PHONY: check
check:
	go fmt ./...
	go vet ./...
	golint -set_exit_status ./...
	golangci-lint run ./...
	go test ./...

.PHONY: gen
gen:
	go generate ./...

.PHONY: run
run: gen check
	go run .
