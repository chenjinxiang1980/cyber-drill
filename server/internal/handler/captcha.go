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

	"drill-system/internal/config"

	"github.com/gin-gonic/gin"
)

// ---- 验证码内存存储（进程内 map + TTL 自动过期清理）----

type captchaEntry struct {
	code    string    // 答案
	expires time.Time // TTL 过期时间
	ip      string    // 绑定的客户端 IP，防止验证码被跨终端复用
}

const captchaStoreMax = 10000

var (
	captchaStore  = make(map[string]*captchaEntry)
	captchaMu     sync.Mutex
	captchaMaxTTL = 300 * time.Second // 实际最长存活上限（秒）
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
func newCaptcha(ttlSec int, ip string) (id, code, imageB64 string) {
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
	now := time.Now()
	if len(captchaStore) >= captchaStoreMax {
		// 先清过期，再按 map 随机淘汰，保证内存占用有上限
		for k, v := range captchaStore {
			if v.expires.Before(now) {
				delete(captchaStore, k)
			}
		}
		for k := range captchaStore {
			if len(captchaStore) < captchaStoreMax {
				break
			}
			delete(captchaStore, k)
		}
	}
	captchaStore[id] = &captchaEntry{code: code, expires: now.Add(time.Duration(ttlSec) * time.Second), ip: ip}
	captchaMu.Unlock()
	return id, code, imageB64
}

// sevenSegDigits 7 段数码管笔画定义（a 上 / b 右上 / c 右下 / d 下 / e 左下 / f 左上 / g 中）。
// 用矢量矩形绘制，避免把题目与答案以 <text> 明文写进 SVG。
var sevenSegDigits = map[rune][]byte{
	'0': {'a', 'b', 'c', 'd', 'e', 'f'},
	'1': {'b', 'c'},
	'2': {'a', 'b', 'g', 'e', 'd'},
	'3': {'a', 'b', 'g', 'c', 'd'},
	'4': {'f', 'g', 'b', 'c'},
	'5': {'a', 'f', 'g', 'c', 'd'},
	'6': {'a', 'f', 'g', 'e', 'c', 'd'},
	'7': {'a', 'b', 'c'},
	'8': {'a', 'b', 'c', 'd', 'e', 'f', 'g'},
	'9': {'a', 'b', 'c', 'd', 'f', 'g'},
}

// segRect 返回单段笔画矩形（14×26 单元格内的相对坐标）。
func segRect(seg byte) string {
	switch seg {
	case 'a':
		return `<rect x="3" y="1" width="8" height="2" rx="1"/>`
	case 'g':
		return `<rect x="3" y="12" width="8" height="2" rx="1"/>`
	case 'd':
		return `<rect x="3" y="23" width="8" height="2" rx="1"/>`
	case 'f':
		return `<rect x="1" y="3" width="2" height="8" rx="1"/>`
	case 'b':
		return `<rect x="11" y="3" width="2" height="8" rx="1"/>`
	case 'e':
		return `<rect x="1" y="14" width="2" height="8" rx="1"/>`
	case 'c':
		return `<rect x="11" y="14" width="2" height="8" rx="1"/>`
	}
	return ""
}

// captchaSVG 生成与深色主题协调的 SVG 验证码。
// 题目与运算符全部用矢量矩形绘制，SVG / base64 中不存在可直接读取的表达式或答案文本。
// 不依赖外部字体文件，画布固定 120×44。
func captchaSVG(expr string) string {
	// 主题亮色系（深蓝黑登录页的亮色呼应）
	var colorA = [][3]int{
		{203, 213, 255}, // #cbd5ff
		{125, 211, 252}, // #7dd3fc
		{167, 139, 250}, // #a78bfa 柔和紫
		{96, 165, 250},  // #60a5fa 天蓝
	}
	idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(colorA))))
	c := colorA[idx.Int64()]
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

	// 干扰线（1-2 条，低透明度）
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

	// 噪点（6-8 个，极低透明度）
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

	// 逐字符绘制：数字用 7 段笔画，+/-/= 用矩形组合。
	compact := strings.ReplaceAll(expr, " ", "")
	chars := []rune(compact)
	const cellW, gap = 14, 2
	total := len(chars)*cellW + (len(chars)-1)*gap
	startX := (120 - total) / 2
	if startX < 2 {
		startX = 2
	}
	var glyphs strings.Builder
	for i, ch := range chars {
		x0 := startX + i*(cellW+gap)
		dy, _ := rand.Int(rand.Reader, big.NewInt(5))
		y0 := 7 + int(dy.Int64())
		rot, _ := rand.Int(rand.Reader, big.NewInt(9)) // -4..4
		glyphs.WriteString(fmt.Sprintf(`<g fill="%s" fill-opacity="0.92" transform="translate(%d,%d) rotate(%d,7,13)">`,
			textColor, x0, y0, int(rot.Int64())-4))
		switch ch {
		case '+':
			glyphs.WriteString(`<rect x="6" y="4" width="2" height="14" rx="1"/>`)
			glyphs.WriteString(`<rect x="2" y="10" width="10" height="2" rx="1"/>`)
		case '-':
			glyphs.WriteString(`<rect x="2" y="10" width="10" height="2" rx="1"/>`)
		case '=':
			glyphs.WriteString(`<rect x="2" y="7" width="10" height="2" rx="1"/>`)
			glyphs.WriteString(`<rect x="2" y="15" width="10" height="2" rx="1"/>`)
		default:
			for _, seg := range sevenSegDigits[ch] {
				glyphs.WriteString(segRect(seg))
			}
		}
		glyphs.WriteString(`</g>`)
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="44" viewBox="0 0 120 44">
<rect width="100%%" height="100%%" fill="rgba(255,255,255,0.03)" rx="4"/>
%s
%s
%s
</svg>`, lines.String(), dots.String(), glyphs.String())
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
	// 绑定 IP 校验：防止验证码被批量抓取后在别的终端复用
	if e.ip != "" && e.ip != c.ClientIP() {
		return false
	}

	// 比对（字符串即算术题的结果字符串）
	if strings.TrimSpace(captchaCode) != e.code {
		return false
	}
	return true
}

// ---- GET /api/auth/captcha ----
// 仅在 cfg.CaptchaTestMode（APP_ENV=test + CAPTCHA_TEST_MODE=true）时额外返回 testCode，
// 供自动化用例走「验证码正确则放行」；生产环境该字段不会出现。
func Captcha(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, code, img := newCaptcha(cfg.CaptchaTTLSeconds, c.ClientIP())
		resp := gin.H{
			"id":    id,
			"image": img,
			"ttl":   cfg.CaptchaTTLSeconds,
		}
		if cfg.CaptchaTestMode {
			resp["testCode"] = code
		}
		c.JSON(http.StatusOK, resp)
	}
}
