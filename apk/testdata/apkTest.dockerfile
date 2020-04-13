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

RUN go test ./apk

# can test a local .apk using:
# apk add --allow-untrusted path/to/some/generated.apk
