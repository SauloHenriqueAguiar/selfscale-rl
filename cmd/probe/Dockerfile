# Proxy de medição (build multi-stage; imagem final mínima).
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY manager/sensors/probe ./manager/sensors/probe
COPY cmd/probe ./cmd/probe
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /probe ./cmd/probe

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /probe /probe
EXPOSE 8080 9100
ENTRYPOINT ["/probe"]
