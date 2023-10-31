FROM ubuntu

ARG HELM_CHART_SOURCE

COPY scripts/install-dependencies.sh scripts/install-dependencies.sh
COPY scripts/create-new-user.sh scripts/create-new-user.sh
COPY tests/dependencies tests/dependencies
COPY tests/vendor tests/vendor
COPY ${HELM_CHART_SOURCE} helm/korifi

RUN apt-get update \
  && apt-get install --yes \
       --no-install-recommends \
       apt-transport-https \
       ca-certificates \
       conntrack \
       gnupg2 \
       curl \
       git \
       sudo \
  && rm -rf /var/lib/apt/lists/*

# helm
RUN echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" >/etc/apt/sources.list.d/helm-stable-debian.list \
  && curl -fsSL https://baltocdn.com/helm/signing.asc | gpg --dearmor >/usr/share/keyrings/helm.gpg

RUN apt-get update \
  && apt-get install --yes \
       helm \
  && rm -rf /var/lib/apt/lists/*

# kubectl
RUN curl -fsSLo /usr/bin/kubectl "https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
  && chmod +x /usr/bin/kubectl
