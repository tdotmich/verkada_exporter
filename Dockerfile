FROM golang:1.27 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /verkada_exporter ./cmd/verkada_exporter

# Distroless: no shell and no package manager. The process runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /verkada_exporter /verkada_exporter

USER nonroot:nonroot
EXPOSE 10061

ENTRYPOINT ["/verkada_exporter"]
