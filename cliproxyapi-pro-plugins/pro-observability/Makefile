GO ?= go
GOOS_NAME := $(shell $(GO) env GOOS)

ifeq ($(GOOS_NAME),darwin)
PLUGIN_EXT := dylib
else ifeq ($(GOOS_NAME),windows)
PLUGIN_EXT := dll
else
PLUGIN_EXT := so
endif

.PHONY: test build clean

test:
	$(GO) test .

build:
	mkdir -p dist
	CGO_ENABLED=1 $(GO) build -buildmode=c-shared -trimpath -o dist/pro-observability.$(PLUGIN_EXT) .

clean:
	$(RM) -r dist
