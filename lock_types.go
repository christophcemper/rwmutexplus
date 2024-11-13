package rwmutexplus

import (
	"fmt"
	"time"
)

// LockType is the type of lock
type LockType int

const (
	ReadLock LockType = iota
	WriteLock
)

// stringer for LockType
func (lt LockType) String() string {
	return []string{"ReadLock", "WriteLock"}[lt]
}

// LockInfo is an interface for both LockRequest and ActiveLock to store lockType, purpose, goRoutineID and callerInfo
type LockInfo interface {
	GetLockTX() LockTX
	GetLockType() LockType
	GetPurpose() string
	GetGoroutineID() uint64
	GetCallerInfo() string
	GetSinceTime() time.Duration
	GetPosition() string
	String() string
}

// LockRequest implements LockInfo
type LockRequest struct {
	lockType    LockType
	purpose     string
	startTime   time.Time
	goroutineID uint64
	callerInfo  string
}

// LockTX is the transaction type for the lock object (LockRequest or ActiveLock)
type LockTX int

const (
	IsLockRequest LockTX = iota
	IsActiveLock
)

// stringer for LockTX
func (lo LockTX) String() string {
	return []string{"REQUEST", "ACTIVE"}[lo]
}

// implements LockInfo
func (lr *LockRequest) GetLockTX() LockTX {
	return IsLockRequest
}

// implements LockInfo
func (lr *LockRequest) GetLockType() LockType {
	return lr.lockType
}

// implements LockInfo
func (lr *LockRequest) GetPurpose() string {
	return lr.purpose
}

// implements LockInfo
func (lr *LockRequest) GetGoroutineID() uint64 {
	return lr.goroutineID
}

// implements LockInfo
func (lr *LockRequest) GetCallerInfo() string {
	return lr.callerInfo
}

// implements LockInfo
func (lr *LockRequest) GetSinceTime() time.Duration {
	return time.Since(lr.startTime)
}

// implements LockInfo
func (lr *LockRequest) GetPosition() string {
	return fmt.Sprintf("for '%s' (goroutine %d)", lr.GetPurpose(), lr.GetGoroutineID())
}

// implements LockInfo
func (lr *LockRequest) String() string {
	return fmt.Sprintf("%s %s %s\n%s", lr.lockType, lr.GetLockTX(), lr.GetPosition(), lr.GetCallerInfo())
}

// ActiveLock tracks an acquired lock
type ActiveLock struct {
	lockType        LockType
	purpose         string
	acquireWaitTime time.Duration
	acquiredAt      time.Time
	goroutineID     uint64
	callerInfo      string
	stack           string
}

// implements LockInfo
func (al *ActiveLock) GetLockTX() LockTX {
	return IsActiveLock
}

// implements LockInfo
func (al *ActiveLock) GetLockType() LockType {
	return al.lockType
}

// implements LockInfo
func (al *ActiveLock) GetPurpose() string {
	return al.purpose
}

// implements LockInfo
func (al *ActiveLock) GetGoroutineID() uint64 {
	return al.goroutineID
}

// implements LockInfo
func (al *ActiveLock) GetSinceTime() time.Duration {
	return time.Since(al.acquiredAt)
}

// implements LockInfo
func (lr *ActiveLock) GetCallerInfo() string {
	return lr.callerInfo
}

// implements LockInfo
func (lr *ActiveLock) GetPosition() string {
	return fmt.Sprintf("for '%s' (goroutine %d)", lr.GetPurpose(), lr.GetGoroutineID())
}

// implements LockInfo
func (al *ActiveLock) String() string {
	return fmt.Sprintf("%s %s %s\n%s", al.lockType, al.GetLockTX(), al.GetPosition(), al.GetCallerInfo())
}

// PrintLockInfo prints the lock info from LockInfo interface
func PrintLockInfo(li LockInfo) {
	fmt.Printf("%s %s for '%s' (goroutine %d)\n%s", li.GetLockTX(), li.GetLockType(), li.GetPurpose(), li.GetGoroutineID(), li.GetCallerInfo())
}

func GetLockInfo(li LockInfo) string {
	return fmt.Sprintf("%s %s for '%s' (goroutine %d)\n%s", li.GetLockTX(), li.GetLockType(), li.GetPurpose(), li.GetGoroutineID(), li.GetCallerInfo())
}
