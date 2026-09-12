FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /build/server .
COPY --from=builder /build/cmd/server/migrations ./cmd/server/migrations
COPY kb ./kb

ENV PORT=8080

EXPOSE 8080

CMD ["./server"]
