# tunnel-client — multi-stage build so the image is self-contained.
# The entrypoint is the binary, so you pass the same flags you'd pass on the
# CLI as the container command:
#   docker run ... tunnel-client:latest --server HOST:9443 --token T --fingerprint F
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tunnel-client ./client

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tunnel-client /tunnel-client
ENTRYPOINT ["/tunnel-client"]
