package handler

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"drill-system/internal/auth"
)

// 登录身份选择的一次性凭据。
//
// 多角色账号第一步凭据校验通过后，服务端返回 ticket + 本人角色快照，
// 第二步凭 ticket + 所选角色换 token。ticket 只在内存中保存 3 分钟，
// redeem 时立即删除（无论后续校验成败），避免被重放。
// 快照里只有角色元数据，不含密码、会话或其它账号信息。
const (
	roleTicketTTL        = 3 * time.Minute
	roleTicketMaxEntries = 20000
	roleTicketExpiresIn  = int(roleTicketTTL / time.Second)
	roleTicketByteLength = 32
)

type roleTicket struct {
	UserID   int64
	Username string
	Roles    []auth.Role
	Expires  time.Time
}

var roleTicketStore = struct {
	sync.Mutex
	m map[string]*roleTicket
}{m: make(map[string]*roleTicket)}

// issueRoleTicket 生成 32 字节随机票据并登记角色快照；失败返回空串。
func issueRoleTicket(userID int64, username string, roles []auth.Role) (string, int) {
	b := make([]byte, roleTicketByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", 0
	}
	ticket := hex.EncodeToString(b)
	now := time.Now()
	roleTicketStore.Lock()
	defer roleTicketStore.Unlock()
	roleTicketSweepLocked(now)
	roleTicketStore.m[ticket] = &roleTicket{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		Expires:  now.Add(roleTicketTTL),
	}
	return ticket, roleTicketExpiresIn
}

// redeemRoleTicket 取出并删除票据（一次性）；过期或不存在返回 false。
func redeemRoleTicket(ticket string) (*roleTicket, bool) {
	if ticket == "" {
		return nil, false
	}
	now := time.Now()
	roleTicketStore.Lock()
	defer roleTicketStore.Unlock()
	t, ok := roleTicketStore.m[ticket]
	if !ok {
		return nil, false
	}
	delete(roleTicketStore.m, ticket)
	if now.After(t.Expires) {
		return nil, false
	}
	return t, true
}

// roleTicketSweepLocked 清理过期票据并在超上限时淘汰最旧条目；调用方必须持锁。
func roleTicketSweepLocked(now time.Time) {
	for k, t := range roleTicketStore.m {
		if now.After(t.Expires) {
			delete(roleTicketStore.m, k)
		}
	}
	if len(roleTicketStore.m) < roleTicketMaxEntries {
		return
	}
	// 超上限：按过期时间从早到晚删除，直到低于阈值。
	for k := range roleTicketStore.m {
		if len(roleTicketStore.m) < roleTicketMaxEntries {
			break
		}
		delete(roleTicketStore.m, k)
	}
}
