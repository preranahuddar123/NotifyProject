FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/notify_app ./cmd

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/notify_app /app/cmd/notify_app
COPY conf/ /app/conf/

WORKDIR /app/cmd

EXPOSE 8081 50051

CMD ["./notify_app"]
