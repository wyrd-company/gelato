FROM golang:1.26-alpine AS build

RUN apk add --no-cache git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gelato ./cmd/soft

FROM alpine:3.24

RUN apk add --no-cache bash git openssh

WORKDIR /gelato
VOLUME /gelato

ENV GELATO_DATA_PATH=/gelato
ENV CI=1

EXPOSE 23231/tcp
EXPOSE 23232/tcp
EXPOSE 23233/tcp
EXPOSE 9418/tcp

COPY --from=build /out/gelato /usr/local/bin/gelato

ENTRYPOINT ["/usr/local/bin/gelato", "serve"]
