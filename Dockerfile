FROM golang:1.17-alpine3.15 as build

WORKDIR /go/src/app

COPY . .

RUN go get -d -v ./...

RUN go install -v ./...

FROM alpine:3.15

WORKDIR /app

COPY --from=build /go/bin/derivative-ms ./derivative-ms

ENTRYPOINT [ "./derivative-ms" ]

CMD [ "-h" ]