FROM ubuntu:22.04

MAINTAINER "Freegle Geeks" <geeks@ilovefreegle.org>

ENV MYSQL_USER=root \
    MYSQL_PASSWORD=secret \
    MYSQL_PROTOCOL=tcp \
    MYSQL_HOST=localhost \
    MYSQL_PORT=3306 \
    MYSQL_DBNAME=iznik \
    IMAGE_DOMAIN=freegle \
    USER_SITE=freegle.test \
    JWT_SECRET=jwtsecret \
    GROUP_DOMAIN=groups.freegle.test

RUN apt update && apt install -y golang-go git \
    && git clone https://github.com/Freegle/iznik-server-go.git

CMD cd iznik-server-go \
  && git pull \
  && go get \
  && echo "Start against DB $MYSQL_HOST:$MYSQL_PORT/$MYSQL_DBNAME" \
  && go run ./main.go