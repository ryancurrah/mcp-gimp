FROM gcr.io/distroless/static
ARG TARGETOS
ARG TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/mcp-gimp /mcp-gimp
# The plug-in runs inside GIMP on the host, so the container only needs to
# reach its socket. GIMP_HOST must point at a reachable address.
ENV GIMP_HOST=host.docker.internal
ENTRYPOINT ["/mcp-gimp"]
