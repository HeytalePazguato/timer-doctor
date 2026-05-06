# Minimal runtime image for timer-doctor.
#
# The binary is produced by goreleaser and copied in from the build
# context — this image isn't used for compilation, just for shipping.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY timer-doctor /usr/local/bin/timer-doctor

ENTRYPOINT ["/usr/local/bin/timer-doctor"]
CMD ["--help"]
