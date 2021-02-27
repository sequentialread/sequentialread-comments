
FROM golang:1.15.2-alpine as build
ARG GOARCH=
ARG GO_BUILD_ARGS=

RUN mkdir /build
WORKDIR /build
RUN apk add --update --no-cache ca-certificates git \
  && go get github.com/boltdb/bolt \
  && go get github.com/gomarkdown/markdown \
  && go get github.com/SYM01/htmlsanitizer \
  && go get github.com/GeorgeMac/idicon/icon \ 
  && go get github.com/GeorgeMac/idicon/colour \
  && go get github.com/xhit/go-simple-mail \
  && go get git.sequentialread.com/forest/pkg-errors
COPY main.go /build/main.go
RUN  go build -v $GO_BUILD_ARGS -o /build/sequentialread-comments .

FROM alpine
WORKDIR /app
COPY --from=build /build/sequentialread-comments /app/sequentialread-comments
#COPY comments.html.gotemplate /app/comments.html.gotemplate
COPY static /app/static
COPY admin.html.gotemplate /app/admin.html.gotemplate
RUN chmod +x /app/sequentialread-comments
ENTRYPOINT ["/app/sequentialread-comments"]