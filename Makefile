# Copyright (C) 2026 Suleyman Poyraz
# SPDX-License-Identifier: GPL-3.0-or-later

BINARY  := docker-tui
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PREFIX  ?= /usr/local

.PHONY: all build run test vet fmt lint install uninstall clean

all: build

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	golangci-lint run

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
