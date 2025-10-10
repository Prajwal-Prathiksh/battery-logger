# Battery Zen Makefile

PREFIX ?= $(HOME)/.local
BINDIR = $(PREFIX)/bin
SERVICEDIR = $(HOME)/.config/systemd/user

BINARY_NAME = battery-zen
SERVICE_NAME = battery-zen.service

.PHONY: help build clean copy-config desktop-icon install install-service logs setup start status stop uninstall

# Show help
help:
	@echo "Available targets:"
	@echo "  build            - Build the binary"
	@echo "  clean            - Remove built binary"
	@echo "  copy-config      - Copy default config to ~/.config/battery-zen (if not exists)"
	@echo "  desktop-icon     - Install desktop icon for Battery Zen"
	@echo "  install          - Install binary to ~/.local/bin"
	@echo "  install-service  - Install and enable systemd service"
	@echo "  logs             - Follow service logs"
	@echo "  setup            - One step setup (install, install-service, desktop-icon, copy-config and start)"
	@echo "  start            - Start the service"
	@echo "  status           - Show service status"
	@echo "  stop             - Stop the service"
	@echo "  uninstall        - Remove everything"
	@echo "  help             - Show this help"

# Build the binary
build:
	go build -o $(BINARY_NAME) ./cmd/battery-zen

# Clean built binary
clean:
	rm -f $(BINARY_NAME)

# Copy default config to user's config directory (skip if exists)
copy-config:
	mkdir -p $(HOME)/.config/battery-zen
	[ -f $(HOME)/.config/battery-zen/config.toml ] || cp internal/config/config.toml $(HOME)/.config/battery-zen/config.toml

# Install desktop icon
desktop-icon:
	mkdir -p $(HOME)/.local/share/applications
	mkdir -p $(HOME)/.local/share/icons
	cp assets/battery-zen.png $(HOME)/.local/share/icons/battery-zen.png
	sed \
		-e 's|@BINDIR@|$(BINDIR)|g' \
		-e 's|@ICONDIR@|$(HOME)/.local/share/icons|g' \
		battery-zen.desktop.in > $(HOME)/.local/share/applications/battery-zen.desktop

# Install binary to ~/.local/bin
install: build
	mkdir -p $(BINDIR)
	cp $(BINARY_NAME) $(BINDIR)/$(BINARY_NAME)

# Install systemd user service
install-service: install
	mkdir -p $(SERVICEDIR)
	sed 's|ExecStart=.*|ExecStart=$(BINDIR)/$(BINARY_NAME) run|' systemd/battery-zen@.service > $(SERVICEDIR)/$(SERVICE_NAME)
	systemctl --user daemon-reload
	systemctl --user enable $(SERVICE_NAME)

# View logs
logs:
	journalctl --user -u $(SERVICE_NAME) -f

# One step setup
setup: install install-service desktop-icon copy-config start

# Start the service
start: install-service
	systemctl --user start $(SERVICE_NAME)

# Check service status
status:
	systemctl --user status $(SERVICE_NAME)

# Stop the service
stop:
	systemctl --user stop $(SERVICE_NAME)

# Uninstall everything
uninstall:
	systemctl --user stop $(SERVICE_NAME) || true
	systemctl --user disable $(SERVICE_NAME) || true
	rm -f $(SERVICEDIR)/$(SERVICE_NAME)
	rm -f $(BINDIR)/$(BINARY_NAME)
	systemctl --user daemon-reload
