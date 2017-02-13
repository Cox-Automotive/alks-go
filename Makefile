package = github.com/Cox-Automotive/alks-go

build:
	go fmt
	go build -v .

test:
	go test -v .

get-deps:
	go install github.com/hashicorp/go-cleanhttp

format:
	go fmt