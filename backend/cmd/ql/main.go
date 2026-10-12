// 青龙面板 / 单机脚本入口：不依赖 MySQL、Redis 和网页，跑一次每日任务后退出。
//
// 环境变量：
//
//	CAIYUN_AUTH   必填，抓包得到的 Authorization（Basic xxx 或只填 xxx）
//	CAIYUN_SKIP   可选，跳过的任务编码，逗号分隔，例如 fun_ai,poster_activity
//	CAIYUN_ONLY   可选，只跑这些任务编码，逗号分隔
//	CAIYUN_DATA   可选，状态目录，默认是程序旁边的 caiyun_data
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/tasks"
	"caiyun/internal/models"
	"caiyun/internal/services"
	"caiyun/internal/version"
)

// 单账号脚本模式不执行的多账号任务（待办：抢兑、139 邮箱互发、活动互助）。
var multiAccountTasks = map[string]bool{"mail_mutual": true, "mutual_assist": true}

const refreshBefore = 5 * 24 * time.Hour

type state struct {
	Phone    string `json:"phone"`
	Auth     string `json:"auth"`
	ExpireAt int64  `json:"expire_at"`
	Updated  string `json:"updated"`
}

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println(version.Version)
		return 0
	}
	if len(os.Args) > 1 && os.Args[1] == "list" {
		for _, d := range services.QLBatchTasks() {
			mark := ""
			if multiAccountTasks[d.Code] {
				mark = "（单账号模式不执行）"
			}
			fmt.Printf("%-24s %s%s\n", d.Code, d.Name, mark)
		}
		return 0
	}

	raw := cleanAuth(os.Getenv("CAIYUN_AUTH"))
	if raw == "" {
		fmt.Println("❌ 没有设置环境变量 CAIYUN_AUTH（抓包得到的 Authorization）")
		return 1
	}
	info, err := auth.ParseToken(raw)
	if err != nil {
		fmt.Println("❌ CAIYUN_AUTH 格式不对:", err)
		return 1
	}

	dataDir := os.Getenv("CAIYUN_DATA")
	if dataDir == "" {
		exe, _ := os.Executable()
		dataDir = filepath.Join(filepath.Dir(exe), "caiyun_data")
	}
	_ = os.MkdirAll(dataDir, 0o700)
	statePath := filepath.Join(dataDir, "state_"+info.Phone+".json")

	// 程序自动续期过的 Authorization 比环境变量里的新，就用新的。
	authFull, expireAt := info.AuthFull, info.Expire
	if st := loadState(statePath); st != nil && st.Phone == info.Phone && st.ExpireAt > expireAt && st.Auth != "" {
		authFull, expireAt = st.Auth, st.ExpireAt
		fmt.Println("使用自动续期后的 Authorization")
	}
	fmt.Printf("账号 %s，Authorization 有效期至 %s\n", maskPhone(info.Phone), time.UnixMilli(expireAt).Format("2006-01-02 15:04"))
	if expireAt <= time.Now().UnixMilli() {
		fmt.Println("⚠️ Authorization 已过期，会尝试续期，失败请重新抓包更新 CAIYUN_AUTH")
	}

	if expireAt <= time.Now().Add(refreshBefore).UnixMilli() {
		if nf, ne, err := refreshAuth(authFull, info.Phone); err != nil {
			fmt.Println("⚠️ Authorization 自动续期失败:", err)
		} else {
			authFull, expireAt = nf, ne
			saveState(statePath, &state{Phone: info.Phone, Auth: nf, ExpireAt: ne, Updated: time.Now().Format(time.RFC3339)})
			fmt.Printf("✅ Authorization 已续期至 %s（保存在 %s，环境变量不用改）\n", time.UnixMilli(ne).Format("2006-01-02"), dataDir)
		}
	}

	// 先确认 Authorization 能用，失效就直接提示，不再跑一遍全部任务。
	if !checkAuth(authFull, info.Phone) {
		fmt.Println("❌ Authorization 已失效，请重新抓包更新 CAIYUN_AUTH")
		return 1
	}

	codes, disabled := selectTasks()
	store := tasks.NewFileStore(filepath.Join(dataDir, "store_"+info.Phone), quietLogger{})
	account := &models.Account{Phone: info.Phone, Auth: authFull, Platform: info.Platform, ExpireAt: expireAt, IsActive: true}
	runner := services.NewTaskRunner(account, tasks.NewScopedStorage(store, "ql:"+info.Phone), auth.NewAuth(corehttp.NewClient()), disabled)

	start := time.Now()
	results := runner.RunSelected(codes)

	var ok, fail, skip int
	lines := []string{}
	for _, r := range results {
		icon := "✅"
		switch r.Status {
		case "failed":
			icon, fail = "❌", fail+1
		case "skipped":
			icon, skip = "⏭", skip+1
		default:
			ok++
		}
		line := fmt.Sprintf("%s %s", icon, services.QLTaskName(r.TaskType))
		if r.Status == "failed" && r.Message != "" {
			line += "：" + oneLine(r.Message)
		}
		lines = append(lines, line)
	}
	fmt.Println("\n======== 移动云盘任务汇总 ========")
	fmt.Printf("账号 %s，成功 %d，失败 %d，跳过 %d，用时 %s\n", maskPhone(info.Phone), ok, fail, skip, time.Since(start).Round(time.Second))
	if total := runner.GetFinalCloudCount(); total > 0 {
		fmt.Printf("本次获得云朵 %d，当前共 %d\n", runner.GetCloudGained(), total)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	if fail > 0 && ok == 0 {
		return 1
	}
	return 0
}

func selectTasks() ([]string, map[string]bool) {
	disabled := map[string]bool{}
	for k := range multiAccountTasks {
		disabled[k] = true
	}
	for _, c := range splitList(os.Getenv("CAIYUN_SKIP")) {
		if n := services.QLNormalizeTask(c); n != "" {
			disabled[n] = true
		}
	}
	codes := []string{}
	only := splitList(os.Getenv("CAIYUN_ONLY"))
	if len(only) > 0 {
		for _, c := range only {
			if n := services.QLNormalizeTask(c); n != "" && !disabled[n] {
				codes = append(codes, n)
			}
		}
		return codes, disabled
	}
	for _, d := range services.QLBatchTasks() {
		if !disabled[d.Code] {
			codes = append(codes, d.Code)
		}
	}
	return codes, disabled
}

func checkAuth(authFull, phone string) bool {
	c := corehttp.NewClient()
	c.SetAuth(strings.TrimPrefix(authFull, "Basic "))
	for i := 0; i < 2; i++ {
		if jwt, _, err := auth.NewAuth(c).GetJWTTokenWithSSOToken(phone); err == nil && jwt != "" {
			return true
		} else if err != nil && strings.Contains(err.Error(), "鉴权失效") {
			fmt.Println("获取 JWT 失败:", err)
			return false
		} else if err != nil {
			fmt.Println("获取 JWT 失败，重试:", err)
		}
		time.Sleep(2 * time.Second)
	}
	return true // 网络类错误交给任务本身重试
}

func refreshAuth(authFull, phone string) (string, int64, error) {
	c := corehttp.NewClient()
	c.SetAuth(strings.TrimPrefix(authFull, "Basic "))
	a := auth.NewAuth(c)
	jwt, _, err := a.GetJWTTokenWithSSOToken(phone)
	if err != nil {
		return "", 0, fmt.Errorf("获取 JWT 失败: %w", err)
	}
	res, err := a.RefreshAuthorization(authFull, phone, services.QLJWTUserDomainID(jwt))
	if err != nil {
		return "", 0, err
	}
	full := res.Auth
	if !strings.HasPrefix(full, "Basic ") {
		full = "Basic " + strings.TrimSpace(full)
	}
	return full, res.ExpireAt, nil
}

func cleanAuth(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'")
	if i := strings.IndexAny(s, "&\n"); i >= 0 { // 只取第一个账号
		s = s[:i]
	}
	if strings.Contains(s, "#") { // 兼容 手机号#Authorization
		parts := strings.SplitN(s, "#", 2)
		s = parts[1]
	}
	return strings.TrimSpace(s)
}

func loadState(p string) *state {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var st state
	if json.Unmarshal(b, &st) != nil {
		return nil
	}
	return &st
}

func saveState(p string, st *state) {
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(p, b, 0o600)
}

func splitList(s string) []string {
	out := []string{}
	for _, v := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '&' }) {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func maskPhone(p string) string {
	if len(p) >= 11 {
		return p[:3] + "****" + p[len(p)-4:]
	}
	return p
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

type quietLogger struct{}

func (quietLogger) Debug(args ...interface{}) {}
func (quietLogger) Error(args ...interface{}) { fmt.Println(args...) }
