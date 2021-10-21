RUN_TEST ?= .
RUN_TEST_FLAGS ?= -v

.PHONY: test
test:
	go test ${RUN_TEST_FLAGS} -count=1 -run=${RUN_TEST} ./...
