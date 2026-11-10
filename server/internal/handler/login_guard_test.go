package handler

import (
	"strconv"
	"testing"
	"time"
)

// TestLoginGuardCountsUntilLocked 回归登录锁定计数：
// 每次请求都会先调用 loginGuardLocked 做预检，它不能把“尚未锁定但已有失败计数”
// 的条目删掉，否则失败次数永远无法累积，锁定形同虚设。
func TestLoginGuardCountsUntilLocked(t *testing.T) {
	key := "unittest|" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer loginGuardSuccess(key)

	for i := 1; i <= 4; i++ {
		if locked, _ := loginGuardLocked(key); locked {
			t.Fatalf("第 %d 次失败前不应锁定", i)
		}
		if locked, _ := loginGuardFailure(key, 5, 15); locked {
			t.Fatalf("第 %d 次失败未达阈值，不应锁定", i)
		}
	}
	// 预检不能清零计数（修复前这里会删除条目）
	if locked, _ := loginGuardLocked(key); locked {
		t.Fatal("未达阈值时不应锁定")
	}
	locked, mins := loginGuardFailure(key, 5, 15)
	if !locked || mins != 15 {
		t.Fatalf("第 5 次失败应锁定 15 分钟，得到 locked=%v mins=%d", locked, mins)
	}
	if locked, mins := loginGuardLocked(key); !locked || mins < 1 {
		t.Fatalf("锁定后预检应返回锁定，得到 locked=%v mins=%d", locked, mins)
	}
}
