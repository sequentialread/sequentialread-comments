
FROM golang:1.15.2-alpine as build
ARG GOARCH=
ARG GO_BUILD_ARGS=

RUN mkdir /build
WORKDIR /build
RUN apk add --update --no-cache ca-certificates git \
  && go get github.com/boltdb/bolt \
  && go get github.com/gomarkdown/markdown \
  && go get github.com/SYM01/htmlsanitizer \
  && go get git.sequentialread.com/forest/pkg-errors
COPY . .
RUN  go build -v $GO_BUILD_ARGS -o /build/sequentialread-comments .

FROM alpine
WORKDIR /app
COPY --from=build /build/sequentialread-comments /app/sequentialread-comments
COPY --from=build /build/comments.html.gotemplate /app/comments.html.gotemplate
COPY --from=build /build/static /app/static
RUN chmod +x /app/sequentialread-comments
ENTRYPOINT ["/app/sequentialread-comments"]