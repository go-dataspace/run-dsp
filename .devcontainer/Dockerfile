FROM golang:1.22

RUN apt-get update && \
    # Go tools:
    # - https://github.com/golang/tools
    # - https://github.com/golang/vscode-go/blob/HEAD/docs/tools.md
    go install -v golang.org/x/tools/gopls@latest && \
    go install github.com/cweill/gotests/gotests@latest && \
    go install github.com/fatih/gomodifytags@latest && \
    go install github.com/josharian/impl@latest && \
    go install github.com/haya14busa/goplay/cmd/goplay@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    go install honnef.co/go/tools/cmd/staticcheck@latest && \
    go install golang.org/x/tools/cmd/godoc@latest && \
    go install gotest.tools/gotestsum@latest && \
    # Go vulnerability checker:
    # https://go.dev/security/vuln/
    go install golang.org/x/vuln/cmd/govulncheck@latest && \
    # Graphviz for visualizing go test pprof profiles.
    apt-get -y install graphviz && \
    # Docker cli
    curl -fsSL https://get.docker.com -o get-docker.sh && \
    sh get-docker.sh

ADD . /workspace
WORKDIR /workspace

CMD ["sleep", "infinity"]

