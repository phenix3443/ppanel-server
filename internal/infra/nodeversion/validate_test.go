package nodeversion

import (
	"testing"
	"time"
)

// 期望版本会被下发到线上节点、触发它自我替换二进制并重启，所以写入前必须校验
// 格式。放进一个随手打错的字符串，节点会拿着它去 GitHub 找不存在的 release
// ——那时故障现场在节点上，离这里很远。
func TestValidateTarget(t *testing.T) {
	valid := []string{
		// 样例都取在 MinSelfManageable 之上——下限本身由
		// TestValidateTargetRejectsUnmanageableVersions 单独锁。
		"v1.1.15", "v1.1.16-rc.1", "v10.0.0",
		"",       // 不干预
		"latest", // 跟随最新；由面板解析成具体 tag 后才下发，节点看不到这个词
	}
	for _, v := range valid {
		if err := ValidateTarget(v); err != nil {
			t.Errorf("ValidateTarget(%q) 应当通过，得到 %v", v, err)
		}
	}
	invalid := []string{
		"1.1.14",            // 缺 v 前缀，和我们的 tag 命名不一致
		"Latest",            // 大小写不同就不是那个约定值
		"v1.1",              // 不完整
		"v1.1.16; rm -rf /", // 会被拼进 URL
		"../../etc/passwd",
		"v1.1.16 ", // 尾随空格，拼 URL 时静默 404
	}
	for _, v := range invalid {
		if err := ValidateTarget(v); err == nil {
			t.Errorf("ValidateTarget(%q) 应当报错", v)
		}
	}
}

// 一个节点会跑什么版本，只由它自己的策略决定——没有全局层，没有回落。
func TestResolve(t *testing.T) {
	withLatest := func(v string) *Cache {
		c := New(0)
		c.value = v
		c.nextFetch = time.Now().Add(time.Hour)
		return c
	}
	cases := []struct{ name, latest, target, want string }{
		{"跟随最新解析成具体 tag", "v2.0.0", AutoLatest, "v2.0.0"},
		{"钉住就原样下发", "v2.0.0", "v1.1.16", "v1.1.16"},
		{"空串表示不自动升级", "v2.0.0", "", ""},
		{"上游还没拉到时不猜版本", "", AutoLatest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withLatest(tc.latest).Resolve(tc.target); got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}
