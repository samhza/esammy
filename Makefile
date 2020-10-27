.POSIX:
.SUFFIXES:

GO = go
RM = rm
GOSRC!=find . -name '*.go'
GOSRC+=go.mod go.sum

all: esammy

esammy: $(GOSRC)
	$(GO) build $(GOFLAGS) ./cmd/esammy

clean:
	$(RM) esammy

.PHONY: all clean
