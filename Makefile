TESTFLAGS=-v -race -count=1 -timeout=360s

# so far only testgoroutineid is implemented 100% PASS, but others can run, too
test: testgoroutineid

test2: testall

testall:
	go test $(TESTFLAGS)

.PHONY: test testunique testleak testall testgoroutineid

# tests of goroutineid v1
testgoroutineid:
	go test $(TESTFLAGS) -run "^TestGoroutineIDUniqueness|TestGoroutineIDStress|TestGoroutineIDMemoryLeak|TestGoroutineIDConcurrentCleanup|TestGoroutineIDWithRace$$"

# tests of goroutineid v2
testallgoroutineid: testunique testleak teststress testcleanup testrace

#single tests of goroutineid
testunique:
	go test $(TESTFLAGS) -run TestGoroutineIDUniqueness

testleak:
	go test $(TESTFLAGS) -run TestGoroutineIDMemoryLeak

teststress:
	go test $(TESTFLAGS) -run TestGoroutineIDStress

testcleanup:
	go test $(TESTFLAGS) -run TestGoroutineIDConcurrentCleanup

testrace:
	go test $(TESTFLAGS) -run TestGoroutineIDWithRace
	