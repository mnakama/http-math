FROM scratch

EXPOSE 8080

ADD http-math /

ENTRYPOINT ["/http-math"]
