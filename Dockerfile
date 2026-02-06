FROM golang:1.24 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gitbrowser main.go

FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY --from=builder /out/gitbrowser /app/gitbrowser
COPY templates /app/templates
COPY static /app/static
COPY repos.json /app/repos.json

ENV GITBROWSER_CONFIG=/app/repos.json
EXPOSE 8080

USER nobody:nobody
ENTRYPOINT ["/app/gitbrowser"]
