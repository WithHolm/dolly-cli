# Dolly in a container: one static program, with the portal built in. ui/dist is in the repo, so no Node is needed.
#
#   docker build -t dolly .
#   docker run --rm -p 127.0.0.1:8650:8650 -v dolly-data:/data dolly
#
# The data is in the volume /data. Publish the port to 127.0.0.1 only, as above: Dolly in a container listens on
# every interface of the container, and has no sign-in of its own.

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/dolly ./cmd/dolly
# the data folder, owned by the user that runs dolly (the image has no shell to do it later)
RUN mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dolly /dolly
COPY --from=build --chown=65532:65532 /out/data /data
ENV DOLLY_DIR=/data \
    DOLLY_HOST=0.0.0.0 \
    DOLLY_PORT=8650
VOLUME /data
EXPOSE 8650
# the image has no shell and no curl: dolly checks itself
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 CMD ["/dolly", "health"]
ENTRYPOINT ["/dolly"]
CMD ["serve"]
