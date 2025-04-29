FROM ubuntu:25.04 AS base

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        g++ \
        build-essential \
    && rm -rf /var/lib/apt/lists/*
ARG APP_DIR=/app
RUN mkdir -p ${APP_DIR}
WORKDIR ${APP_DIR}

CMD ["tail", "-f", "/dev/null"]
