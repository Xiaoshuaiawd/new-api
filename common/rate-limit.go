package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.allowRequestLocked(key, maxRequestNum, duration, true)
}

// Allow checks whether a request is allowed without recording it.
// Parameter duration's unit is seconds.
func (l *InMemoryRateLimiter) Allow(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.allowRequestLocked(key, maxRequestNum, duration, false)
}

func (l *InMemoryRateLimiter) allowRequestLocked(key string, maxRequestNum int, duration int64, record bool) bool {
	if maxRequestNum == 0 {
		return true
	}
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			if record {
				*queue = append(*queue, now)
			}
			return true
		} else {
			if now-(*queue)[0] >= duration {
				if record {
					*queue = (*queue)[1:]
					*queue = append(*queue, now)
				}
				return true
			} else {
				return false
			}
		}
	} else {
		if !record {
			return true
		}
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}
