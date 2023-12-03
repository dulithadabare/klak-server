# syntax=docker/dockerfile:1

FROM golang:1.17-alpine as builder
# CI Commit Sha value
ARG CI_COMMIT_SHA
ARG CI_COMMIT_TITLE
ARG CI_JOB_ID
ARG VERSION=NotSet
# Create a directory inside the builder to be used as the default destination for all subsequent commands
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
# Copy our source code into the image.
COPY *.go ./
#  compile our application
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \ 
-ldflags="-X 'main.Version=${VERSION}' -X 'main.BuildCommit=${CI_COMMIT_SHA}' -X 'main.BuildCommitTitle=${CI_COMMIT_TITLE}' -X 'main.BuildJobId=${CI_JOB_ID}' -X 'main.BuildTime=$(date)' "

# Use the alpine package manager to fetch the current ca-certificates package
FROM alpine:latest as certs
RUN apk --update add ca-certificates

# Use busybox as the base image
FROM busybox
# Copy the downloaded artifact into the image based on busybox
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy over firebase service key
COPY hamuwemu-app-firebase-adminsdk-serviceAccountKey.json /home/

# Copy over apns key
COPY AuthKey_S9DN84Q7KS.p8 /home/

# Copy over the executable file
COPY --from=builder ./app/hamuwemu-server /home/
# Run the executable file
# CMD /home/hamuwemu-server
CMD [ "/home/hamuwemu-server" ]