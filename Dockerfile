FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/clickshield ./cmd/server
# IP to AS database (public domain, refreshed hourly upstream); every build
# ships the current copy.
ARG ASN_DATABASE_URL=https://iptoasn.com/data/ip2asn-combined.tsv.gz
RUN wget -q -O /out/ip2asn.tsv.gz "$ASN_DATABASE_URL" && gzip -t /out/ip2asn.tsv.gz

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/clickshield /app/clickshield
COPY --from=build /out/ip2asn.tsv.gz /app/ip2asn.tsv.gz
USER app
EXPOSE 8080
ENTRYPOINT ["/app/clickshield"]
