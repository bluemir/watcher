##@ Run

run: build/$(APP_NAME) ## Run application
	$< -vv --include "**.go" -- echo 1
dev-run: build/$(APP_NAME) ## If detect file change, automatically rebuild.
	while true; do \
		$< --include "**.go" -- $(MAKE) test run; \
		echo "hit ^C again to quit" && sleep 1 \
	; done


reset: ## Kill all make process. Use when dev-run stuck.
	ps -e | grep make | grep -v grep | awk '{print $$1}' | xargs kill

WATCHED_FILES+=$(MAKEFILE_LIST)

.watched:
	@echo $(WATCHED_FILES) | tr " " "\n"

tools: build/tools/entr
build/tools/entr:
	@which $(notdir $@) || (echo "see http://eradman.com/entrproject")

.PHONY: run dev-run reset .watched
