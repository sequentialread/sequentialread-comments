
FROM golang:1.16-alpine as build
ARG GOARCH=
ARG GO_BUILD_ARGS=

RUN mkdir /build
WORKDIR /build
RUN apk add --update --no-cache ca-certificates git \
  && go get github.com/boltdb/bolt \
  && go get github.com/gomarkdown/markdown \
  && go get github.com/sym01/htmlsanitizer \
  && go get github.com/xhit/go-simple-mail \
  && go get git.sequentialread.com/forest/pkg-errors
COPY main.go /build/main.go
COPY go.mod /build/go.mod
COPY go.sum /build/go.sum
RUN  go get && go build -v $GO_BUILD_ARGS -o /build/sequentialread-comments .

FROM alpine
WORKDIR /app
COPY --from=build /build/sequentialread-comments /app/sequentialread-comments
#COPY comments.html.gotemplate /app/comments.html.gotemplate
COPY static /app/static
COPY admin.html.gotemplate /app/admin.html.gotemplate
RUN chmod +x /app/sequentialread-comments
ENTRYPOINT ["/app/sequentialread-comments"]