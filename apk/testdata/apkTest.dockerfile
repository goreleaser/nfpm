# run using:
#docker build -t dantest -f ./apk/testdata/apkTest.dockerfile . && docker run -it dantest

#FROM alpine
FROM golang:1.13-alpine

WORKDIR /go/src/app

RUN apk add --no-cache gcc musl-dev

#RUN apk add --no-cache openssh-client
#RUN addgroup -S appgroup && adduser -S appuser -G appgroup
#USER appuser
#RUN ssh-keygen -t rsa -b 4096 -m pem -N "" -f /home/appuser/.ssh/id_rsa

USER root

COPY . .

RUN skipVerifyInfo=true go test ./apk
RUN ls -Rla ./apk/testdata/workdir/test-run*
RUN apk add -vvv --allow-untrusted ./apk/testdata/workdir/test-run*/apkToCreate.apk

# can test using:
# apk add --allow-untrusted ./apk/testdata/workdir/test-run*/apkToCreate.apk
