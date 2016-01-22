GOPATH := $(shell pwd)
.PHONY: clean test

all:
	@GOPATH=$(GOPATH) go install golcx

clean:
	@rm -fr bin pkg
