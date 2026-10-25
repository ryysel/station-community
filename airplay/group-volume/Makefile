CC ?= cc
CFLAGS ?= -O2 -g -Wall -Wextra -Werror -Wno-misleading-indentation
PKG_CONFIG ?= pkg-config
DBUS_FLAGS := $(shell $(PKG_CONFIG) --cflags --libs dbus-1)
LDLIBS := $(DBUS_FLAGS) -lcrypto

.PHONY: all test clean
all: ap2-group-volume
ap2-group-volume: group-volume.c
	$(CC) $(CFLAGS) $< -o $@ $(LDLIBS)
test-group-volume: test-group-volume.c group-volume.c
	$(CC) $(CFLAGS) $< -o $@ $(LDLIBS)
test: test-group-volume
	./test-group-volume
clean:
	rm -f ap2-group-volume test-group-volume
