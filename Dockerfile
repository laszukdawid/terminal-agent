FROM golang:1.21-alpine

# Prepare testing environment
RUN apk add --no-cache bash

WORKDIR /agent

# Copy the agent binary
COPY test/ .
