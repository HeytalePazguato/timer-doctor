FROM scratch

COPY timer-doctor /usr/local/bin/timer-doctor

ENTRYPOINT ["/usr/local/bin/timer-doctor"]
