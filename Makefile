build:
	mkdir -p functions
    GOOS=linux GOARCH=amd64 go mod download github.com/Azure/go-ansiterm
    GOOS=linux GOARCH=amd64 go build -o functions/main main.go