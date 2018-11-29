FROM docker.io/gordons/qpid-proton:0.26.0

ADD send /
ADD recv /
ENTRYPOINT ["/recv"]
CMD ["-a", "test"]
