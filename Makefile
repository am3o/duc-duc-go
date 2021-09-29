vendor:
	go mod vendor

test: vendor ## Runs the tests
	go test $(GOFLAGS) -cover -race -short -v $(shell go list $(GOFLAGS) ./... | grep -v /vendor/ | grep -v /test/)
