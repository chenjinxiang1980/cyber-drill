package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- 验证码内存存储（进程内 map + TTL 自动过期清理）----

type captchaEntry struct {
	code    string    // 答案
	expires time.Time // TTL 过期时间
}

var (
	captchaStore   = make(map[string]*captchaEntry)
	captchaMu      sync.Mutex
	captchaMaxTTL  = 300 * time.Second // 实际最长存活上限（秒）
)

func init() {
	// 后台清理协程
	go captchaGC()
}

func captchaGC() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		captchaMu.Lock()
		for id, e := range captchaStore {
			if e.expires.Before(now) {
				delete(captchaStore, id)
			}
		}
		captchaMu.Unlock()
	}
}

// generateCaptchaID 生成 16 字符随机 id
func generateCaptchaID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// newCaptcha 生成一道简单算术题（≤100 的加法/减法），返回 id、答案、SVG image
func newCaptcha(ttlSec int) (id, code, imageB64 string) {
	id = generateCaptchaID()

	// 生成题目：简单加法 a + b 或减法 a - b（保证正数结果）
	a, _ := rand.Int(rand.Reader, big.NewInt(50))
	b, _ := rand.Int(rand.Reader, big.NewInt(50))
	a2, _ := rand.Int(rand.Reader, big.NewInt(2))

	var expr string
	var result int
	if a2.Int64() == 0 {
		// 加法
		result = int(a.Int64()) + int(b.Int64())
		expr = fmt.Sprintf("%d + %d", a.Int64(), b.Int64())
	} else {
		// 减法：确保 a >= b
		av, bv := int(a.Int64()), int(b.Int64())
		if av < bv {
			av, bv = bv, av
		}
		result = av - bv
		expr = fmt.Sprintf("%d - %d", av, bv)
	}
	code = strconv.Itoa(result)

	// 生成 SVG（内嵌 expr，无外部字体依赖）
	svg := captchaSVG(expr)
	imageB64 = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))

	// 存储
	if ttlSec <= 0 || ttlSec > int(captchaMaxTTL.Seconds()) {
		ttlSec = 120
	}
	captchaMu.Lock()
	captchaStore[id] = &captchaEntry{code: code, expires: time.Now().Add(time.Duration(ttlSec) * time.Second)}
	captchaMu.Unlock()
	return id, code, imageB64
}

// captchaSVG 生成与深色主题协调的 SVG 验证码（内嵌题目文字 + 轻微干扰）
// 不依赖外部字体文件，画布固定 120×44
func captchaSVG(expr string) string {
	// 主题亮色系（深蓝黑登录页的亮色呼应）
	// 在 #cbd5ff 与 #7dd3fc 之间随机挑一个 + 轻微随机偏移，保证每次风格统一但有变化
	var colorA = [][3]int{
		{203, 213, 255}, // #cbd5ff
		{125, 211, 252}, // #7dd3fc
		{167, 139, 250}, // #a78bfa 柔和紫
		{96, 165, 250},  // #60a5fa 天蓝
	}
	idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(colorA))))
	c := colorA[idx.Int64()]
	// 给每通道 ±20 微调，让每次不那么死板
	for i := 0; i < 3; i++ {
		d, _ := rand.Int(rand.Reader, big.NewInt(41))
		if d.Int64() >= 20 {
			c[i] += int(d.Int64()) - 20
		} else {
			c[i] -= 20 - int(d.Int64())
		}
		if c[i] < 120 {
			c[i] = 120
		}
		if c[i] > 255 {
			c[i] = 255
		}
	}
	textColor := fmt.Sprintf("rgb(%d,%d,%d)", c[0], c[1], c[2])

	// 干扰线（1-2 条，低透明度，位置避开中央字符区）
	var lines strings.Builder
	nLines := 1
	if r, _ := rand.Int(rand.Reader, big.NewInt(2)); r.Int64() == 0 {
		nLines = 2
	}
	for i := 0; i < nLines; i++ {
		x1, _ := rand.Int(rand.Reader, big.NewInt(15))
		y1, _ := rand.Int(rand.Reader, big.NewInt(40))
		x2, _ := rand.Int(rand.Reader, big.NewInt(15))
		y2, _ := rand.Int(rand.Reader, big.NewInt(40))
		lines.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-opacity="0.18" stroke-width="1.2"/>`,
			x1.Int64(), y1.Int64()+2, x2.Int64()+105, y2.Int64()+2, textColor))
	}

	// 噪点（6-10 个，极低透明度）
	var dots strings.Builder
	nDots := 6
	if r, _ := rand.Int(rand.Reader, big.NewInt(5)); r.Int64() >= 2 {
		nDots = 8
	}
	for i := 0; i < nDots; i++ {
		dx, _ := rand.Int(rand.Reader, big.NewInt(120))
		dy, _ := rand.Int(rand.Reader, big.NewInt(44))
		dots.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="%s" fill-opacity="0.14"/>`,
			dx.Int64(), dy.Int64(), textColor))
	}

	// 主文本：轻微随机偏移，避免被识别脚本吃死
	dyOff, _ := rand.Int(rand.Reader, big.NewInt(7)) // 0-6px 上移
	rotate := 0
	if r, _ := rand.Int(rand.Reader, big.NewInt(3)); r.Int64() == 1 {
		rotate = int(r.Int64()) * 2 - 3 // -3 或 3 度
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="44" viewBox="0 0 120 44">
<!-- 背景：极低不透明度白色，让登录页背景隐约透出 -->
<rect width="100%%" height="100%%" fill="rgba(255,255,255,0.03)" rx="4"/>
<!-- 干扰线 -->
%s
<!-- 噪点 -->
%s
<!-- 题目文字：居中、主题色、轻微旋转偏移 -->
<g transform="translate(0,%d) rotate(%d,60,22)">
<text x="60" y="28" text-anchor="middle" font-size="16" font-family="ui-monospace, Consolas, monospace" fill="%s" font-weight="600" letter-spacing="0">%s = ?</text>
</g>
</svg>`, lines.String(), dots.String(), dyOff.Int64()-3, rotate, textColor, expr)
	return svg
}

// verifyCaptcha 校验验证码：成功或过期即作废
func verifyCaptcha(c *gin.Context, captchaID, captchaCode string) bool {
	captchaMu.Lock()
	e, ok := captchaStore[captchaID]
	if !ok {
		captchaMu.Unlock()
		return false
	}
	// 立即删除（一次性使用）
	delete(captchaStore, captchaID)
	captchaMu.Unlock()

	// 过期检查
	if e.expires.Before(time.Now()) {
		return false
	}

	// 比对（字符串即算术题的结果字符串）
	if strings.TrimSpace(captchaCode) != e.code {
		return false
	}
	return true
}

// ---- GET /api/auth/captcha ----
func Captcha(ttlSec int) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _, img := newCaptcha(ttlSec)
		c.JSON(http.StatusOK, gin.H{
			"id":   id,
			"image": img,
			"ttl":  ttlSec,
		})
	}
}
