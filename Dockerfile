FROM golang:1.27.0 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /pg-es-proxy ./cmd/pg-es-proxy

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /pg-es-proxy /pg-es-proxy

ENTRYPOINT ["/pg-es-proxy"]
