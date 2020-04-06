# run using:
#docker build -t dantest -f ./apk/testdata/apkTest.dockerfile . && docker run -it dantest

#FROM alpine
FROM golang:1.13-alpine

WORKDIR /go/src/app
COPY . .

RUN apk add --no-cache rpm

RUN apk add --no-cache gcc musl-dev openssh-client

#ENTRYPOINT ["/nfpm"]

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser
#RUN ssh-keygen -t rsa -b 4096 -N "" -f /home/appuser/.ssh/id_rsa
RUN ssh-keygen -t rsa -b 4096 -m pem -N "" -f /home/appuser/.ssh/id_rsa

USER root
#RUN go test ./apk

# can test using:
# apk add --allow-untrusted ./apk/apkToCreate.apk
