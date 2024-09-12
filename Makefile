build:
	mkdir -p functions
	GOOS=linux GOARCH=amd64 go mod tidy
	GOOS=linux GOARCH=amd64 go build -o functions/online functions-src/online.go