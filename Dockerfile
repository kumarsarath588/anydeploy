## Builder stage
FROM golang:1.17 as builder

COPY . /app

WORKDIR /app

# Download go deps
RUN go mod download

# Resulting binary will not be linked to any C libraries
ENV CGO_ENABLED=0

RUN GOOS=linux go build -o anydeploy main.go

## Container build stage
FROM alpine:3.14.3

# Install curl dependency required for healthcheck
# Create user/group admin to run container as admin user
RUN apk --no-cache add curl && \
apk del musl busybox  alpine-keys  apk-tools && \
addgroup -S admin && \
adduser -S admin -G admin

# Default Environment variable;
# If below env will be overriden when passed with -e during docker run
ENV APP_DB_HOST=mysql \
    APP_DB_PORT=3306 \
    APP_DB_NAME=anydeploy \
    AMQP_CONNECTION="amqp://guest:guest@rabbitmq:5672/" \
    AMQP_QUEUE_NAME=provision_queue

# Run app as admin user
USER admin

WORKDIR /app

# Copy binary from builder state
COPY --from=builder /app/anydeploy .
COPY scripts/entrypoint.sh .

EXPOSE 8080

#HEALTHCHECK --interval=30s --timeout=3s --retries=5 --start-period=15s CMD curl -f http://localhost:8080/health || exit 1

CMD ["/app/anydeploy"]

ENTRYPOINT [ "/app/entrypoint.sh" ]