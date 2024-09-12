build:
    GOOS=linux GOARCH=amd64 go mod tidy
    GOOS=linux GOARCH=amd64 go build -o netlify/functions/online ./netlify/functions/online.go