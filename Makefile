TESTFLAGS1=-race -count=1 -timeout=360s
TESTFLAGS=$(TESTFLAGS1)

# so far only testgoroutineid is implemented 100% PASS, but others can run, too
test: testgoroutineid

testv: 
	$(MAKE) testgoroutineid TESTFLAGS="-v $(TESTFLAGS1)"

test2: testall

testall:
	go test $(TESTFLAGS)

testallv:
	go test $(TESTFLAGS) -v


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

#benchmarks of a core lib like goroutineid are important
benchmark: benchmarkgoroutineid

benchmarkgoroutineid:
	go test -benchmem -run="^$$" -bench="^BenchmarkGoroutineID$$" $(TESTFLAGS)
