# For deploying on Netlify
go version

# Don't understand why these are necessary, because (presumaly) I don't understand at least one of Go modules/Netlify
go mod download github.com/aws/aws-lambda-go
go mod download github.com/awslabs/aws-lambda-go-api-proxy
go mod download github.com/freegle/iznik-server-go
go mod download github.com/go-sql-driver/mysql
go mod download github.com/gofiber/fiber/v2

go build -o functions/gateway main.go