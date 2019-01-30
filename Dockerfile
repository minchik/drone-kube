FROM alpine:3.4 as ca-source

RUN apk update && apk add ca-certificates


FROM golang:1.11-alpine as build-backend

ARG SOURCE_COMMIT

WORKDIR /go/src/github.com/minchik/drone-kube
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s -X main.revision=${SOURCE_COMMIT}"


FROM scratch

COPY --from=ca-source /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build-backend /go/src/github.com/minchik/drone-kube/drone-kube /bin/
ENTRYPOINT ["/bin/drone-kube"]
