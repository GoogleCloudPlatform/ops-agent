.PHONY: update-go-module
update-go-module: GO_MOD_VERSION ?= latest
update-go-module:
ifndef GO_MOD
	@echo "Missing a GO_MOD to update"
else
	@FOUND_MOD=$$(go list -m all | grep '^$(GO_MOD)') && \
		[ -n "$$FOUND_MOD" ] && \
		go get -u $(GO_MOD)@$(GO_MOD_VERSION) || echo "module $(GO_MOD) not found in ${PWD}"
endif
