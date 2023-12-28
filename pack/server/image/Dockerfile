#
# Release
#
FROM --platform=$TARGETPLATFORM alpine:3.19.0

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV DEBIAN_FRONTEND=noninteractive
RUN set -eo pipefail; \
    apk add -U --no-cache \
      ca-certificates \
      openssl \
      curl unzip \
      git \
    ; \
    rm -rf /var/cache/apk/*

# set locale
RUN set -eo pipefail; \
    apk add -U --no-cache \
      tzdata \
    ; \
    rm -rf /var/cache/apk/*
ENV LANG='en_US.UTF-8' \
    LANGUAGE='en_US:en' \
    LC_ALL='en_US.UTF-8'

EXPOSE 80 443
VOLUME /var/run/hermitcrab
COPY /image/ /
COPY /build/server-${TARGETOS}-${TARGETARCH} /usr/bin/hermitcrab
ENV _RUNNING_INSIDE_CONTAINER_="true" \
    TF_PLUGIN_MIRROR_DIR="/usr/share/terraform/providers/plugins"
CMD ["hermitcrab", "--log-debug", "--log-verbosity=5"]
