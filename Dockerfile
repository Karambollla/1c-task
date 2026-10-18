FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/edition-server ./cmd/edition-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/edition-client ./cmd/edition-client

FROM debian:bookworm-slim AS server
RUN useradd --system --create-home edition && \
    mkdir /data && \
    chown edition:edition /data
COPY --from=build /out/edition-server /usr/local/bin/edition-server
USER edition
VOLUME ["/data"]
EXPOSE 9000
ENTRYPOINT ["edition-server"]
CMD ["serve", "--db", "/data/edition.db", "--listen", ":9000"]

FROM debian:bookworm-slim AS client
COPY --from=build /out/edition-client /usr/local/bin/edition-client
ENTRYPOINT ["edition-client"]
