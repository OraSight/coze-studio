# syntax=docker/dockerfile:1.7

# Stage 1: Builder for Go application
FROM golang:1.24-alpine AS builder

ARG ALPINE_MIRROR=mirrors.aliyun.com
ARG GOPROXY_URL=https://goproxy.cn,direct
ARG GOSUMDB_URL=sum.golang.google.cn

WORKDIR /app

ENV GOPROXY=${GOPROXY_URL} \
    GOSUMDB=${GOSUMDB_URL}

# Install build dependencies for Go
RUN --mount=type=cache,target=/var/cache/apk \
    sed -i "s|https://dl-cdn.alpinelinux.org/alpine|https://${ALPINE_MIRROR}/alpine|g" /etc/apk/repositories && \
    apk update && \
    apk add --no-cache git gcc libc-dev

# Copy go.mod and go.sum first to leverage Docker cache
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the entire backend source code
COPY backend/ ./

# Build the Go application
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /app/opencoze main.go


# Stage 2: Final image
FROM alpine:3.22.0

ARG ALPINE_MIRROR=mirrors.aliyun.com
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn

WORKDIR /app

ENV PIP_INDEX_URL=${PIP_INDEX_URL} \
    PIP_TRUSTED_HOST=${PIP_TRUSTED_HOST} \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_ROOT_USER_ACTION=ignore

# Install runtime dependencies for Go app and base for Python
# pax-utils for scanelf, python3 for running Python
# bind-tools for nslookup etc., file for debugging file types
RUN --mount=type=cache,target=/var/cache/apk \
    sed -i "s|https://dl-cdn.alpinelinux.org/alpine|https://${ALPINE_MIRROR}/alpine|g" /etc/apk/repositories && \
    apk update && \
    apk add --no-cache ca-certificates pax-utils python3 bind-tools file deno curl

RUN --mount=type=cache,target=/root/.cache/deno \
    deno run -A jsr:@langchain/pyodide-sandbox -c "print('Hello, World')"

# Install Python build dependencies, create venv, install packages, then remove build deps
RUN --mount=type=cache,target=/var/cache/apk \
    --mount=type=cache,target=/root/.cache/pip \
    apk add --no-cache --virtual .python-build-deps build-base py3-pip git python3-dev && \
    python3 -m venv --copies --upgrade-deps /app/.venv && \
    . /app/.venv/bin/activate && \
    pip install --prefer-binary \
        urllib3==1.26.16 \
        h11==0.16.0 \
        httpx==0.28.1 \
        pillow==11.2.1 \
        pdfplumber==0.11.7 \
        python-docx==1.2.0 \
        numpy==2.3.1 && \
    apk del .python-build-deps


# Copy the built Go binary from the builder stage
COPY --from=builder /app/opencoze /app/opencoze

# Copy Python application scripts
COPY backend/infra/document/parser/impl/builtin/parse_pdf.py /app/parse_pdf.py
COPY backend/infra/document/parser/impl/builtin/parse_docx.py /app/parse_docx.py
COPY backend/infra/coderunner/impl/script/sandbox.py /app/sandbox.py


# Copy static resources
# COPY backend/static /app/resources/static/
COPY backend/conf /app/resources/conf/
COPY docker/.env.example /app/.env
# COPY docker/.env.ve /app/.env
# COPY docker/cert.pem /app/cert.pem
# COPY docker/key.pem /app/key.pem

# Set PATH to prioritize venv's binaries
ENV PATH="/app/.venv/bin:${PATH}"

# Ensure python scripts and venv executables are executable
RUN chmod +x /app/parse_pdf.py /app/parse_docx.py  && \
    find /app/.venv/bin -type f -exec chmod +x {} \;


EXPOSE 8888

ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

CMD ["/app/opencoze"]
