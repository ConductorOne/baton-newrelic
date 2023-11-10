FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-newrelic"]
COPY baton-newrelic /