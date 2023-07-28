FROM ubuntu:22.04

ENV MYSQL_USER=root \
    MYSQL_PASSWORD=iznik \
    MYSQL_PROTOCOL=tcp \
    MYSQL_HOST=localhost \
    MYSQL_PORT=3306 \
    MYSQL_DBNAME=iznik \
    IMAGE_DOMAIN=http://apiv1.localhost \
    USER_SITE=freegle.test \
    JWT_SECRET=jwtsecret \
    GROUP_DOMAIN=groups.freegle.test

RUN apt update && apt install -y golang-go git \
    && git clone https://github.com/Freegle/iznik-server-go.git

CMD cd iznik-server-go \
  && git pull \
  && go get \
  && echo "Start against DB $MYSQL_HOST:$MYSQL_PORT/$MYSQL_DBNAME with user $MYSQL_USER password $MYSQL_PASSWORD" \
  && while true; do go run main.go; sleep 1; done