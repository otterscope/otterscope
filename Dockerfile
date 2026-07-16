# Built by GoReleaser: the context contains the prebuilt static binary.
# FROM scratch is a product constraint (ADR-0004): no runtime dependencies
# beyond the binary, CA certs (for LLM-judge evals), and the data volume.
FROM alpine:3.22 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY otterscope /otterscope
VOLUME /data
EXPOSE 8317 4318
ENTRYPOINT ["/otterscope"]
CMD ["serve", "-db", "/data/otterscope.db", "-listen", ":8317", "-otlp", ":4318"]
