FROM golang:1.17-alpine as builder

RUN apk add --no-cache git

# Enable go modules.
ENV GO111MODULE=on

WORKDIR /workdir

# Install go modules.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy application files.
COPY . .

# Build binaries.
RUN go build -o /usr/local/bin/server cmd/greeter_server/main.go
RUN go build -o /usr/local/bin/client cmd/greeter_client/main.go

RUN go build -o /usr/local/bin/xds cmd/xds/main.go

FROM alpine:3.16 as server

COPY --from=builder /usr/local/bin/server /usr/local/bin/server

ENV NAME="{{SERVER_ID}}"

ENTRYPOINT ["server"]

FROM alpine:3.16 as client

COPY --from=builder /usr/local/bin/client /usr/local/bin/client

ENV NAME="{{CLIENT_ID}}"

ENTRYPOINT ["client"]

FROM alpine:3.16 as xds

COPY --from=builder /usr/local/bin/xds /usr/local/bin/xds

ENTRYPOINT ["xds"]
