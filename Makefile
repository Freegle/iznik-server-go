build:
	mkdir -p functions
	GOOS=linux GOARCH=amd64 go mod tidy
	GOOS=linux GOARCH=amd64 go build -o functions-src/online functions/online.go