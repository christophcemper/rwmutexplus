// registry.go to keep track of all the RWMutexPlus instances
package rwmutexplus

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	// Global registry of all RWMutexPlus instances
	globalRegistry = &registry{
		mutexes: make(map[string]*RWMutexPlus),
	}
)

type registry struct {
	sync.RWMutex
	mutexes map[string]*RWMutexPlus
}

// register adds a mutex to the global registry
func (r *registry) register(m *RWMutexPlus) {
	r.Lock()
	defer r.Unlock()
	r.mutexes[m.name] = m
}

// unregister removes a mutex from the global registry
func (r *registry) unregister(name string) {
	r.Lock()
	defer r.Unlock()
	delete(r.mutexes, name)
}

// getAllMutexes returns a sorted list of all registered mutexes
func (r *registry) getAllMutexes() []*RWMutexPlus {
	r.RLock()
	defer r.RUnlock()

	mutexes := make([]*RWMutexPlus, 0, len(r.mutexes))
	for _, m := range r.mutexes {
		mutexes = append(mutexes, m)
	}

	// Sort by name for consistent output
	sort.Slice(mutexes, func(i, j int) bool {
		return mutexes[i].name < mutexes[j].name
	})

	return mutexes
}

// DumpAllLockInfo prints detailed information about all the RWMutexPlus instances
// -  for all mutexes
// -  for all mutexes with pending read locks
// -  for all mutexes with pending write locks
// -  for all mutexes with active read locks
// -  for all mutexes with active write locks
// -  for all mutexes with statistics
// Add these types and functions to registry.go

type LockFilter uint8

const (
	ShowPendingReads LockFilter = 1 << iota
	ShowPendingWrites
	ShowActiveReads
	ShowActiveWrites
)

// DumpAllLockInfo prints detailed information about all RWMutexPlus instances
// filters parameter controls which lock types to show
func DumpAllLockInfo(filters ...LockFilter) string {
	var output strings.Builder
	mutexes := globalRegistry.getAllMutexes()

	// Combine all filters
	var combinedFilter LockFilter
	if len(filters) == 0 {
		// If no filters specified, show everything
		combinedFilter = ShowPendingReads | ShowPendingWrites | ShowActiveReads | ShowActiveWrites
	} else {
		for _, f := range filters {
			combinedFilter |= f
		}
	}

	output.WriteString("=== RWMutexPlus Global Status ===\n\n")
	output.WriteString(fmt.Sprintf("Total Registered Mutexes: %d\n", len(mutexes)))
	output.WriteString(fmt.Sprintf("Active Filters: %s\n\n", describeFilters(combinedFilter)))

	// Show filtered pending locks
	if combinedFilter&(ShowPendingReads|ShowPendingWrites) != 0 {
		output.WriteString("--- Mutexes with Pending Locks ---\n")
		for _, m := range mutexes {
			pendingInfo := m.GetPendingLocksInfo()
			hasPendingLocks := false

			var pendingDetails strings.Builder
			if combinedFilter&ShowPendingReads != 0 && len(pendingInfo["pending_reads"]) > 0 {
				hasPendingLocks = true
				pendingDetails.WriteString(fmt.Sprintf("  Pending Reads: %d\n", len(pendingInfo["pending_reads"])))
				for _, read := range pendingInfo["pending_reads"] {
					pendingDetails.WriteString(fmt.Sprintf("    - Purpose: %s, Waiting: %v, Goroutine: %d, Location: %s\n",
						read["purpose"], read["waiting_for"], read["goroutine_id"], read["caller_info"]))
				}
			}
			if combinedFilter&ShowPendingWrites != 0 && len(pendingInfo["pending_writes"]) > 0 {
				hasPendingLocks = true
				pendingDetails.WriteString(fmt.Sprintf("  Pending Writes: %d\n", len(pendingInfo["pending_writes"])))
				for _, write := range pendingInfo["pending_writes"] {
					pendingDetails.WriteString(fmt.Sprintf("    - Purpose: %s, Waiting: %v, Goroutine: %d, Location: %s\n",
						write["purpose"], write["waiting_for"], write["goroutine_id"], write["caller_info"]))
				}
			}

			if hasPendingLocks {
				output.WriteString(fmt.Sprintf("• %s:\n", m.name))
				output.WriteString(pendingDetails.String())
			}
		}
		output.WriteString("\n")
	}

	// Show filtered active locks
	if combinedFilter&(ShowActiveReads|ShowActiveWrites) != 0 {
		output.WriteString("--- Mutexes with Active Locks ---\n")
		for _, m := range mutexes {
			activeInfo := m.GetActiveLocksInfo()
			hasActiveLocks := false

			var activeDetails strings.Builder
			if combinedFilter&ShowActiveReads != 0 && len(activeInfo["read_locks"]) > 0 {
				hasActiveLocks = true
				activeDetails.WriteString(fmt.Sprintf("  Active Reads: %d\n", len(activeInfo["read_locks"])))
				for _, read := range activeInfo["read_locks"] {
					activeDetails.WriteString(fmt.Sprintf("    - Purpose: %s, Held: %v, Goroutine: %d, Location: %s\n",
						read["purpose"], read["held_for"], read["goroutine_id"], read["caller_info"]))
				}
			}
			if combinedFilter&ShowActiveWrites != 0 && len(activeInfo["write_locks"]) > 0 {
				hasActiveLocks = true
				activeDetails.WriteString(fmt.Sprintf("  Active Writes: %d\n", len(activeInfo["write_locks"])))
				for _, write := range activeInfo["write_locks"] {
					activeDetails.WriteString(fmt.Sprintf("    - Purpose: %s, Held: %v, Goroutine: %d, Location: %s\n",
						write["purpose"], write["held_for"], write["goroutine_id"], write["caller_info"]))
				}
			}

			if hasActiveLocks {
				output.WriteString(fmt.Sprintf("• %s:\n", m.name))
				output.WriteString(activeDetails.String())
			}
		}
		output.WriteString("\n")
	}

	return output.String()
}

// Helper function to describe active filters for output
func describeFilters(filter LockFilter) string {
	if filter == 0 {
		return "None"
	}

	var filters []string
	if filter&ShowPendingReads != 0 {
		filters = append(filters, "PendingReads")
	}
	if filter&ShowPendingWrites != 0 {
		filters = append(filters, "PendingWrites")
	}
	if filter&ShowActiveReads != 0 {
		filters = append(filters, "ActiveReads")
	}
	if filter&ShowActiveWrites != 0 {
		filters = append(filters, "ActiveWrites")
	}
	return strings.Join(filters, ", ")
}
