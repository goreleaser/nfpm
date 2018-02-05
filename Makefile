check:
	bandep --ban github.com/tj/assert,github.com/alecthomas/template

test:
	go test -v ./...
