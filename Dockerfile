ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/hitch ./cmd/hitch \
	&& CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/hitch-client ./cmd/hitch-client

FROM alpine:3.22
RUN apk add --no-cache ca-certificates nodejs tzdata \
	&& addgroup -S hitch \
	&& adduser -S -G hitch -h /var/lib/hitch hitch \
	&& mkdir -p /var/lib/hitch/.config/hitch /usr/share/hitch \
	&& chown -R hitch:hitch /var/lib/hitch
COPY --from=build /out/hitch /usr/local/bin/hitch
COPY --from=build /out/hitch-client /usr/local/bin/hitch-client
COPY docker/scripts/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh
ENV HOME=/var/lib/hitch \
	HITCH_CONFIG=/var/lib/hitch/.config/hitch/config.toml \
	HITCH_SERVER_HOST=0.0.0.0
USER hitch
EXPOSE 8799
VOLUME ["/var/lib/hitch/.config/hitch"]
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["serve"]
