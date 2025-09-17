FROM golang:1.23

WORKDIR /app

ENV MYSQL_USER=root \
    MYSQL_PASSWORD=iznik \
    MYSQL_PROTOCOL=tcp \
    MYSQL_HOST=percona \
    MYSQL_PORT=3306 \
    MYSQL_DBNAME=iznik \
    IMAGE_DOMAIN=apiv1.localhost \
    USER_SITE=freegle.localhost \
    JWT_SECRET=jwtsecret \
    GROUP_DOMAIN=groups.freegle.test

RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

# Install go-swagger for API documentation generation (using newer version to fix Go 1.23 compatibility)
RUN go install github.com/go-swagger/go-swagger/cmd/swagger@latest

COPY . .
RUN go mod tidy

# Make generate-swagger.sh executable and generate swagger documentation during build
RUN chmod +x generate-swagger.sh && ./generate-swagger.sh

EXPOSE 8192

CMD echo "Start against DB $MYSQL_HOST:$MYSQL_PORT/$MYSQL_DBNAME with user $MYSQL_USER password $MYSQL_PASSWORD" \
  && while true; do go run main.go >> /tmp/iznik_api.out 2>&1; sleep 1; done