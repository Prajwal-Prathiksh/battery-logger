# Battery Logger Makefile

PREFIX ?= $(HOME)/.local
BINDIR = $(PREFIX)/bin
SERVICEDIR = $(HOME)/.config/systemd/user

BINARY_NAME = battery-logger
SERVICE_NAME = battery-logger.service

.PHONY: all build install install-service start stop status clean uninstall

all: build

# Build the binary
build:
	go build -o $(BINARY_NAME) ./cmd/battery-logger

# Install binary to ~/.local/bin
install: build
	mkdir -p $(BINDIR)
	cp $(BINARY_NAME) $(BINDIR)/$(BINARY_NAME)

# Install systemd user service
install-service: install
	mkdir -p $(SERVICEDIR)
	sed 's|ExecStart=.*|ExecStart=$(BINDIR)/$(BINARY_NAME) run|' systemd/battery-logger@.service > $(SERVICEDIR)/$(SERVICE_NAME)
	systemctl --user daemon-reload
	systemctl --user enable $(SERVICE_NAME)

# Start the service
start: install-service
	systemctl --user start $(SERVICE_NAME)

# Stop the service
stop:
	systemctl --user stop $(SERVICE_NAME)

# Check service status
status:
	systemctl --user status $(SERVICE_NAME)

# View logs
logs:
	journalctl --user -u $(SERVICE_NAME) -f

# One-command setup: build, install, and start
setup: start

# Clean built binary
clean:
	rm -f $(BINARY_NAME)

# Uninstall everything
uninstall:
	systemctl --user stop $(SERVICE_NAME) || true
	systemctl --user disable $(SERVICE_NAME) || true
	rm -f $(SERVICEDIR)/$(SERVICE_NAME)
	rm -f $(BINDIR)/$(BINARY_NAME)
	systemctl --user daemon-reload

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  install       - Install binary to ~/.local/bin"
	@echo "  install-service - Install and enable systemd service"
	@echo "  start         - Start the service"
	@echo "  stop          - Stop the service"
	@echo "  status        - Show service status"
	@echo "  logs          - Follow service logs"
	@echo "  setup         - Build, install, and start (one-command setup)"
	@echo "  clean         - Remove built binary"
	@echo "  uninstall     - Remove everything"
	@echo "  help          - Show this help"
