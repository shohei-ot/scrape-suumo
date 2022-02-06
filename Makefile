BINDIR:=bin

GOOS:=$(shell echo $GOOS)
GOARCH:=$(shell echo $GOARCH)
GOARM:=$(shell echo $GOARM)

build:
	GOOS=$(GOOS) ;\
	GOARCH=$(GOARCH) ;\
	GOARM=$(GOARM) ;\
	go build -o ./$(BINDIR)/scrape-suumo ./cmd/scrape-suumo.go

build-armv7:
	$(MAKE) build GOOS=linux GOARCH=arm GOARM=7

