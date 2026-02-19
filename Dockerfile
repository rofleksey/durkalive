FROM golang:1.25-alpine AS apiBuilder
WORKDIR /opt
RUN apk update && apk add --no-cache make
COPY go.mod go.sum /opt/
RUN go mod download
COPY . /opt/
RUN make build
ARG GIT_TAG
ARG GIT_COMMIT
ARG GIT_COMMIT_DATE
RUN make build GIT_TAG=${GIT_TAG} GIT_COMMIT=${GIT_COMMIT} GIT_COMMIT_DATE=${GIT_COMMIT_DATE}

FROM alpine
ENV ENVIRONMENT=production
ENV CGO_ENABLED=0
WORKDIR /opt
RUN apk update && \
    apk add --no-cache curl ca-certificates && \
    update-ca-certificates
COPY --from=apiBuilder /opt/durkalive /opt/durkalive
CMD [ "./durkalive" ]
