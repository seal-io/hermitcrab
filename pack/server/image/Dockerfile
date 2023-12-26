#
# Fetch
#
FROM --platform=$TARGETPLATFORM hashicorp/terraform:1.5.7 AS fetch

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

RUN set -eo pipefail; \
    apk add -U --no-cache \
      ca-certificates \
      curl openssh \
      unzip \
      git \
      jq \
    ; \
    rm -rf /var/cache/apk/*

ENV TF_LOG="ERROR"
WORKDIR /workspace

# download templates
RUN set -eo pipefail; \
    echo "walrus-catalog walrus-catalog-sandbox" | tr -s '[:blank:]' '\n' | \
    while read -r org _; do \
      curl -sSL "https://api.github.com/orgs/$org/repos" | jq -r '.[].name' | \
      while read -r repo _; do \
        git clone "https://github.com/$org/$repo" "$org"_"$repo" --depth 1; \
      done; \
    done

# mirror plugins
## cache plugins to reduce network latency
ENV TF_PLUGIN_CACHE_DIR="/workspace/.terraform.d/plugin-cache" \
    TF_PLUGIN_MIRROR_DIR="/workspace/.terraform.d/plugins"
RUN set -eo pipefail; \
    mkdir -p $TF_PLUGIN_CACHE_DIR; \
    mkdir -p $TF_PLUGIN_MIRROR_DIR; \
    echo -e "provider_installation {\n \
      filesystem_mirror {\n \
        path = \"$TF_PLUGIN_MIRROR_DIR\"\n \
      }\n \
      direct {} \n \
    }\n" > /root/.terraformrc && \
    find . -maxdepth 1 -type d -name 'walrus-catalog*' -exec sh -c 'terraform -chdir="$1" init && terraform -chdir="$1" providers mirror $TF_PLUGIN_MIRROR_DIR' _ {} \;
## remove non-plugin files to prevent annoying message
RUN set -eo pipefail; \
    find $TF_PLUGIN_MIRROR_DIR -type f ! -name "terraform-provider-*" -delete

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

# get terraform plugins
COPY --from=fetch /workspace/.terraform.d/plugins /usr/share/terraform/providers/plugins

EXPOSE 80 443
VOLUME /var/run/hermitcrab
COPY /image/ /
COPY /build/server-${TARGETOS}-${TARGETARCH} /usr/bin/hermitcrab
ENV _RUNNING_INSIDE_CONTAINER_="true" \
    TF_PLUGIN_MIRROR_DIR="/usr/share/terraform/providers/plugins"
CMD ["hermitcrab", "--log-debug", "--log-verbosity=4"]
