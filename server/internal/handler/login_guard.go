package handler

import (
	"strings"
	"sync"
	"time"
)

// 登录失败锁定：键 = 账号 + 客户端 IP。
// 旧实现把失败次数写进 users.login_attempts，任何人只需知道用户名就能
// 从任意 IP 把账号锁死（拒绝服务）。改为内存双维度计数后，攻击者只能
// 锁定“自己这个 IP + 目标账号”的组合，不会影响其他办公终端。
type loginGuardEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

const loginGuardMaxEntries = 20000

var loginGuardStore = struct {
	sync.Mutex
	m map[string]*loginGuardEntry
}{m: make(map[string]*loginGuardEntry)}

func loginGuardKey(username, ip string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + strings.TrimSpace(ip)
}

// loginGuardLocked 返回 (是否锁定, 剩余分钟)。
func loginGuardLocked(key string) (bool, int) {
	loginGuardStore.Lock()
	defer loginGuardStore.Unlock()
	e, ok := loginGuardStore.m[key]
	if !ok {
		return false, 0
	}
	now := time.Now()
	if e.lockedUntil.After(now) {
		mins := int((e.lockedUntil.Sub(now) + time.Minute - 1) / time.Minute)
		if mins < 1 {
			mins = 1
		}
		return true, mins
	}
	// 锁定期已过：重置该键
	delete(loginGuardStore.m, key)
	return false, 0
}

// loginGuardFailure 记录一次密码失败；达到阈值时返回 (true, 锁定分钟)。
func loginGuardFailure(key string, maxFailures, lockMinutes int) (bool, int) {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if lockMinutes <= 0 {
		lockMinutes = 15
	}
	loginGuardStore.Lock()
	defer loginGuardStore.Unlock()
	now := time.Now()
	loginGuardSweepLocked(now)
	e, ok := loginGuardStore.m[key]
	if !ok {
		e = &loginGuardEntry{}
		loginGuardStore.m[key] = e
	}
	e.failures++
	e.lastSeen = now
	if e.failures >= maxFailures {
		e.lockedUntil = now.Add(time.Duration(lockMinutes) * time.Minute)
		e.failures = 0
		return true, lockMinutes
	}
	return false, 0
}

// loginGuardSuccess 登录成功后清除该键。
func loginGuardSuccess(key string) {
	loginGuardStore.Lock()
	delete(loginGuardStore.m, key)
	loginGuardStore.Unlock()
}

// loginGuardSweepLocked 清理已过期条目并在超上限时淘汰最旧条目；调用方必须持锁。
func loginGuardSweepLocked(now time.Time) {
	if len(loginGuardStore.m) < loginGuardMaxEntries {
		for k, e := range loginGuardStore.m {
			if !e.lockedUntil.IsZero() && e.lockedUntil.Before(now) {
				delete(loginGuardStore.m, k)
			}
		}
		return
	}
	for k, e := range loginGuardStore.m {
		if !e.lockedUntil.After(now) || now.Sub(e.lastSeen) > 24*time.Hour {
			delete(loginGuardStore.m, k)
		}
	}
	for k := range loginGuardStore.m {
		if len(loginGuardStore.m) < loginGuardMaxEntries {
			break
		}
		delete(loginGuardStore.m, k)
	}
}
