ROOT_FOLDER 		:= $(shell git rev-parse --show-toplevel)
include 			$(ROOT_FOLDER)/.scripts/make/golang.mk

SHA1				:= $(shell git rev-parse --verify HEAD)
SHA1_SHORT			:= $(shell git rev-parse --verify --short HEAD)
VERSION				:= $(shell cat VERSION.txt)
INTERNAL_BUILD_ID	:= $(shell [ -z "${GITHUB_RUN_NUMBER}" ] && echo "0" || echo ${GITHUB_RUN_NUMBER})
PWD					:= $(shell pwd)
VERSION_HASH		:= ${VERSION}.${INTERNAL_BUILD_ID}-${SHA1_SHORT}
CI					:= $(shell echo ${CI})

ENVIRONMENT 		?= local


#
# Default Goals
#
.DEFAULT_GOAL		:= VERSION

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Returns a list of all the make goals
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# VERSION
#
#
version: ## Returns version for build
	@echo "Build Version: v$(VERSION_HASH)"

#
# Utils
#

check-ENVIRONMENT: ## Check that the $ENVIRONMENT ENV variable is set.
ifndef ENVIRONMENT
	$(error ENVIRONMENT is undefined)
endif

check-APP: ## Check that the $APP ENV variable is set. Needed for golang and ui commands
ifndef APP
	$(error APP is undefined)
endif
