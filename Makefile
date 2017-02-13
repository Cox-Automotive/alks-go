package = github.com/Cox-Automotive/alks-go

format:
	go fmt

build:
	go fmt
	go build -v .

test:
	go test -v .