FROM golang:1.24.0 AS build

ARG GOOS
ARG GOARCH

ENV CGO_ENABLED=0
ENV GOSUMDB=off
ENV GOPROXY=direct

WORKDIR /work
COPY . /work

RUN GOOS=${GOOS} GOARCH=${GOARCH} go build -o bin/kube-tailor .

# ---
FROM scratch AS run

COPY --from=build /work/bin/kube-tailor /usr/local/bin/

CMD ["kube-tailor"]
