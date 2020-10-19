.POSIX:
.SUFFIXES:

GO = go
RM = rm

all: esammy

esammy:
	$(GO) build $(GOFLAGS) ./cmd/esammy

clean:
	$(RM) esammy

.PHONY: all clean
