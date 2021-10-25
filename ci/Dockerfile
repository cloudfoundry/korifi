FROM docker:latest

RUN apk add --no-cache \
  bash \
  curl \
  alpine-sdk \
  nodejs # required by github actions like checkout

ARG GO_VERSION=1.17
RUN mkdir -p /usr/local/go && \
  curl -sL "https://dl.google.com/go/go$GO_VERSION.linux-amd64.tar.gz" | tar xz -C "/usr/local"
ENV PATH /usr/local/go/bin:$PATH
ENV GOPATH /go
ENV PATH $GOPATH/bin:$PATH

ARG HNC_VERSION=v0.8.0
RUN curl -sL "https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-$HNC_VERSION/kubectl-hns_linux_amd64" -o /usr/local/bin/kubectl-hns && \
  chmod +x "/usr/local/bin/kubectl-hns"

ARG KBLD_VERSION=v0.31.0
RUN  curl -sL "https://github.com/vmware-tanzu/carvel-kbld/releases/download/$KBLD_VERSION/kbld-linux-amd64" -o /usr/local/bin/kbld && \
  chmod +x /usr/local/bin/kbld

ARG KAPP_VERSION=v0.42.0
RUN  curl -sL "https://github.com/vmware-tanzu/carvel-kapp/releases/download/$KAPP_VERSION/kapp-linux-amd64" -o /usr/local/bin/kapp && \
  chmod +x /usr/local/bin/kapp


ARG GINKGO_VERSION=v1.16.5
RUN go install "github.com/onsi/ginkgo/ginkgo@$GINKGO_VERSION" "github.com/onsi/gomega/...@$GINKGO_VERSION"

# ARG KIND_VERSION=v0.11.1
# RUN go install sigs.k8s.io/kind@$KIND_VERSION
