# HTTP 抓屏方案 A：Chromium 企业策略 `OverrideSecurityRestrictionsOnInsecureOrigin`

> 适用对象：在浏览器里打开演练系统（`http://192.168.97.131:8090`）后，希望「上传证据附件 → 屏幕截图」可用。
> 结论：在 **Chrome / Edge** 上通过企业策略把该地址标记为「可信来源（安全上下文）」，即可恢复 `getDisplayMedia`，**不需要改服务端、不需要 HTTPS、不需要重启现网容器**。
> 本机实测：Chrome 156 / Edge 157 均已生效，并实际抓取到一帧 1920×1080 的屏幕画面。

---

## 1. 为什么必须做这一步

`navigator.mediaDevices.getDisplayMedia()`（以及 `getUserMedia`）只在**安全上下文**（`window.isSecureContext === true`）中可用。浏览器把 `http://内网IP:端口` 一律视为**非安全上下文**，此时：

- `navigator.mediaDevices` 直接是 `undefined`；
- 页面里的 `getDisplayMedia` 调用必然报错，这不是前端 Bug，无法用 JS 绕过。

实测基线（本机 Chrome，未做任何配置）：

```
URL                http://192.168.97.131:8090/login.html
isSecureContext    false
navigator.mediaDevices   undefined
getDisplayMedia    undefined
crypto.subtle      false
```

方案 A 的做法就是让浏览器「把指定的这个 origin 当成安全来源」。它与启动参数 `--unsafely-treat-insecure-origin-as-secure=<地址>` 等价，微软官方文档明确说明该策略会**覆盖**同名命令行参数。

---

## 2. 适用版本

| 浏览器 | 支持版本 | 说明 | 本机实测 |
|---|---|---|---|
| Google Chrome | Windows **69+** | 旧策略 `UnsafelyTreatInsecureOriginAsSecure` 自 Chrome 65 提供，M69 起弃用并改用 `OverrideSecurityRestrictionsOnInsecureOrigin` | **156.0.8078.80 ✔** |
| Microsoft Edge | Windows **77+** | 微软官方策略文档标注 Windows 77 起支持 | **157.0.4341.42 ✔** |
| Firefox / Safari | — | 没有等价的「按站点放行安全上下文」企业策略，**本方案不覆盖**，这类终端只能走 HTTPS（方案 B） | 未验证 |
| 国产双核浏览器（360/QQ 等） | 不确定 | 内核够新时可能读取 Chrome 策略，但不保证；关键终端建议统一用 Chrome/Edge 或改 HTTPS | 未验证 |

> 策略是**按注册表位置读取**的，与浏览器是否安装成便携版无关；但企业定制版内核是否实现该策略无法保证，需单独抽查。

---

## 3. 策略语法（按官方定义，不要臆造写法）

- 策略名：`OverrideSecurityRestrictionsOnInsecureOrigin`
- 类型：**List of strings（字符串列表）**
- Windows 注册表写法：**建一个与策略同名的子键**，子键下**每个条目一个值**，值名依次写 `1`、`2`、`3`…，类型 `REG_SZ`，值就是 origin 字符串。
- **不能**用分号/空格把多个地址拼成一个值——这种写法对列表型策略无效，浏览器会读不到。
- 值必须是 **origin**：带协议、可带端口、**不能带路径**，且与实际访问地址**精确匹配**（`http` 与 `https`、`8090` 与非 `8090` 是不同 origin）。
- 同一台机器上 Chrome 与 Edge 是两个独立的注册表位置，要分别写入。

两个位置：

```
HKLM\SOFTWARE\Policies\Google\Chrome\OverrideSecurityRestrictionsOnInsecureOrigin
HKLM\SOFTWARE\Policies\Microsoft\Edge\OverrideSecurityRestrictionsOnInsecureOrigin
```

各自子键下：

```
名称        类型      数据
1          REG_SZ    http://192.168.97.131:8090
2          REG_SZ    http://另一个地址:端口        ← 需要更多地址时才加
```

> `http://localhost:8090`、`http://127.0.0.1:8090` 本身就被浏览器视为「可信」，**不需要**加进策略列表。
> 本次默认只放行现网地址 `http://192.168.97.131:8090`；如有第二个常用地址（例如院内 DNS 名），在子键下加 `2`，不要改写成一行拼接。

---

## 4. 交付物

| 文件 | 用途 | 用法 |
|---|---|---|
| `install/windows/Enable-DrillsInsecureOrigin.reg` | 纯注册表文件（HKLM，Chrome + Edge 各 1 条） | 右键「合并」，或用 `reg import` |
| `install/windows/Enable-DrillsInsecureOrigin.cmd` | 一键入口（自动提权 + 执行 + 提示必须完全退出浏览器） | **右键「以管理员身份运行」**，或双击后在 UAC 点「是」 |
| `install/windows/Enable-DrillsInsecureOrigin.ps1` | 实际执行体，支持撤销/自定义 | `powershell -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 [-Remove] [-Origin ...] [-CurrentUser]` |
| `docs/verify/policy-*.png` | 策略页截图证据 | 见第 6 节 |
| `docs/verify/chrome-policy-insecure-origin-verify.json` | 各轮验证的原始数值 | 见第 6 节 |

### ps1 参数

```powershell
# 应用（默认 HKLM，非管理员会自动弹 UAC 提权）
powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1

# 追加第二个地址（每个地址一条，值名 1、2…）
powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 `
    -Origin http://192.168.97.131:8090,http://drill.example.local:8090

# 撤销
powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 -Remove

# 只写当前用户（HKCU），不写 HKLM
powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 -CurrentUser
```

> 注意：脚本重复执行时会**先清空该子键下已有的数字值再写**，避免残留旧地址。
> 本机实测发现 `HKCU\SOFTWARE\Policies` 对普通用户只有「读取」权限，`-CurrentUser` 同样会因权限被拒；若要用 HKCU，仍需管理员。

---

## 5. 单机操作步骤（非域机器）

1. 把 `install/windows/` 三个文件复制到目标机器（同目录）。
2. **右键 `Enable-DrillsInsecureOrigin.cmd` → 以管理员身份运行**；UAC 点「是」。
3. 看到 `[OK] Google Chrome：1 条` / `[OK] Microsoft Edge：1 条` 即写入成功。
4. **完全退出浏览器**：关闭所有窗口，并确认任务管理器里没有 `chrome.exe` / `msedge.exe` 残留（含托盘常驻进程）。脚本会检测并提示；必要时：

   ```cmd
   taskkill /f /im chrome.exe /im msedge.exe
   ```

5. 重新打开浏览器，地址栏输入 `chrome://policy`（Edge 为 `edge://policy`），搜索 `OverrideSecurityRestrictionsOnInsecureOrigin`，状态应为 **正常 / OK**。
6. 打开 `http://192.168.97.131:8090`，F12 → Console：

   ```js
   isSecureContext                                  // 期望 true
   typeof navigator.mediaDevices.getDisplayMedia    // 期望 "function"
   ```

### 只在没有 cmd/ps1 的情况下用 regedit 手工导入

- 方式一：双击 `Enable-DrillsInsecureOrigin.reg` → 弹确认框 → 「是」。
- 方式二：命令行（管理员）：

  ```cmd
  reg add "HKLM\SOFTWARE\Policies\Google\Chrome\OverrideSecurityRestrictionsOnInsecureOrigin" /v 1 /t REG_SZ /d "http://192.168.97.131:8090" /f
  reg add "HKLM\SOFTWARE\Policies\Microsoft\Edge\OverrideSecurityRestrictionsOnInsecureOrigin" /v 1 /t REG_SZ /d "http://192.168.97.131:8090" /f
  ```

- 方式三：`regedit` → 定位到第 3 节的两个位置 → 新建「项」（名字与策略同名）→ 在项内新建「字符串值」，名称 `1`，数据填地址。

导入后**同样必须完全退出浏览器**再启动。

---

## 6. 生效条件与验证

### 生效条件（缺一不可）

1. 浏览器版本达到第 2 节要求；
2. 值类型是 `REG_SZ`，值名是 `1`、`2`…（不是把地址当值名、也不是分号拼接）；
3. 写在了浏览器实际读取的位置（Chrome 读 `...\Google\Chrome\...`，Edge 读 `...\Microsoft\Edge\...`）；
4. **浏览器进程完全重启**——该策略 `Dynamic Policy Refresh = No`，热改不生效；
5. 策略页状态为 **正常 / OK**；
6. 访问地址与策略值**精确一致**（协议、主机、端口都相同，不能带路径）。

### 本机实测结果（2026-11-07）

| 场景 | isSecureContext | getDisplayMedia | 策略页状态 | 实际抓屏 |
|---|---|---|---|---|
| 基线（无策略、无参数） | `false` | `undefined` | 未设置 | 不可用 |
| 仅命令行参数 `--unsafely-treat-insecure-origin-as-secure=...` | `true` | `function` | 未设置 | —（该轮只验证判定） |
| **仅注册表策略（无任何安全相关启动参数）** | `true` | `function` | **正常** | — |
| **注册表策略 + Chrome** | `true` | `function` | **正常**，值 `["http://192.168.97.131:8090"]`，来源 平台，对象 本机，级别 强制 | **成功**：track `screen:0:0`，`live`，1920×1080，耗时 287ms，帧约 675KB |
| **注册表策略 + Edge** | `true` | `function` | **确定（OK）** | **成功**：track `screen:0:0`，`live`，1920×1080，耗时 260ms |
| 执行 `-Remove` 后重启浏览器 | `false` | `undefined` | 未设置 | 回到不可用 |
| 重新应用后 | `true` | `function` | 正常 | **成功**（帧 SHA1 `fedfb20b…`） |

策略页截图：

- `docs/verify/policy-chrome-unset-before.png` —— 配置前，策略「未设置」
- `docs/verify/policy-chrome-ok.png` —— Chrome 配置后，状态「正常」
- `docs/verify/policy-chrome-ok-noflags.png` —— **未传任何 Chromium 参数**时策略仍生效（排除命令行干扰）
- `docs/verify/policy-edge-ok.png` —— Edge 配置后，状态「确定」
- `docs/verify/policy-chrome-unset-after-remove.png` —— 撤销后恢复「未设置」
- 原始数值：`docs/verify/chrome-policy-insecure-origin-verify.json`

注册表读取证据：

```
HKEY_LOCAL_MACHINE\SOFTWARE\Policies\Google\Chrome\OverrideSecurityRestrictionsOnInsecureOrigin
    1    REG_SZ    http://192.168.97.131:8090

HKEY_LOCAL_MACHINE\SOFTWARE\Policies\Microsoft\Edge\OverrideSecurityRestrictionsOnInsecureOrigin
    1    REG_SZ    http://192.168.97.131:8090
```

> 抓屏验证方式：在**真实站点页面上下文**里调用 `getDisplayMedia({video:true})`，用测试脚本的 `--auto-select-desktop-capture-source` 自动选中整屏（等价于点掉浏览器的选择器），拿到 `live` 的屏幕轨道并把首帧画到 canvas 取 SHA1。
> 未在现网业务里点「上传证据附件 → 屏幕截图」按钮，是为了避免往现网演练库写入测试附件；浏览器能力本身已端到端验证。

---

## 7. 撤销方法

推荐（会自动删两个子键并提示重启浏览器）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Enable-DrillsInsecureOrigin.ps1 -Remove
```

或手工（管理员 CMD）：

```cmd
reg delete "HKLM\SOFTWARE\Policies\Google\Chrome\OverrideSecurityRestrictionsOnInsecureOrigin" /f
reg delete "HKLM\SOFTWARE\Policies\Microsoft\Edge\OverrideSecurityRestrictionsOnInsecureOrigin" /f
```

或 `regedit` 里直接删除这两个同名子键（只删子键，不要删 `...\Policies\Google\Chrome` 或 `...\Policies\Microsoft\Edge` 本身，其它策略可能在里面）。

撤销后**必须完全退出浏览器再启动**，验证：`chrome://policy` 里该策略变回「未设置」，页面 `isSecureContext` 回到 `false`。

---

## 8. 批量下发建议

- **域环境（推荐）**：
  1. 首选官方 ADMX/ADML 模板，在「计算机配置 → 策略 → 管理模板」里按策略名 `OverrideSecurityRestrictionsOnInsecureOrigin` 定位并启用，把地址逐条加入列表；
  2. 没有 ADMX 时，用「计算机配置 → 首选项 → Windows 设置 → 注册表」推送：分别为 Chrome 与 Edge 新建「注册表项」，键路径见第 3 节，值为 REG_SZ、名称 `1`；
  3. 只链接到**演练终端所在的 OU**，不要下发到全院。
- **非域环境**：用 Intune / SCCM / PDQ / 自建分发把 `.cmd` 作为管理员任务推送；或用「计算机启动脚本」以系统权限调用 `.ps1`（用户登录脚本没有管理员权限，写 HKLM 会失败）。
- **下发后抽查**（抽 1~2 台即可）：
  ```cmd
  reg query "HKLM\SOFTWARE\Policies\Google\Chrome\OverrideSecurityRestrictionsOnInsecureOrigin" /s
  reg query "HKLM\SOFTWARE\Policies\Microsoft\Edge\OverrideSecurityRestrictionsOnInsecureOrigin" /s
  ```
  再打开 `chrome://policy` 确认状态「正常」。
- **撤销同样要批量做**：策略是长期生效的，演练结束后若改成 HTTPS，应通过同一 GPO/分发通道删除，避免长期豁免。

---

## 9. 风险与边界

1. **只把「指定的这个地址」当成可信来源**，不影响其它站点，也不需要关闭任何安全防护；但该 origin 上的资源会被按「安全来源」处理（例如混合内容判定放宽），因此列表要**最小化**，不要写成整个网段或 `http://*`。
2. 只解决浏览器的安全上下文门槛；如果以后上 HTTPS（方案 B），应删除本策略，避免两套豁免长期并存。
3. 覆盖不到 Firefox / Safari / 部分国产内核；这些终端要么换浏览器，要么等 HTTPS。
4. 必须重启浏览器，**不能**指望刷新页面生效；用户没完全退出（托盘常驻）时会表现为「策略页 OK 但页面仍不可用」，这是最常见的失败原因。
5. 若策略值写成 `http://192.168.97.131:8090/`（带斜杠）或漏了端口，**不会匹配**，属于精确匹配。

---

## 10. 当前状态（本机）

- 已在本机 `HKLM` 写入 Chrome 与 Edge 各 1 条 `http://192.168.97.131:8090`，并实测生效（见第 6 节）。
- 为验证撤销路径，曾执行一次 `-Remove`（确认恢复为非安全上下文），随后已重新应用，当前处于**已启用**状态。
- 本轮**未**重启现网容器、未改后端、未推仓库/镜像/tag；现网 `http://192.168.97.131:8090` 全程正常响应（HTTP 200）。
- 如需在本机回滚：执行第 7 节的撤销命令，完全退出浏览器后即可恢复。
