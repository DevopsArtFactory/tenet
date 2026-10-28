# Supply a reviewed, digest-pinned Python 3.12+ slim base when building.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG VERSION=baseline
ARG FAIL_HEALTH=false
ENV VERSION=${VERSION} FAIL_HEALTH=${FAIL_HEALTH} PYTHONDONTWRITEBYTECODE=1
WORKDIR /app
COPY server.py .
USER 65534:65534
EXPOSE 8080
CMD ["python", "server.py"]
