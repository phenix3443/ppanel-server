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
		"v1.1.14", "v1.1.14-rc.1", "v10.0.0",
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
		"v1.1.14; rm -rf /", // 会被拼进 URL
		"../../etc/passwd",
		"v1.1.14 ", // 尾随空格，拼 URL 时静默 404
	}
	for _, v := range invalid {
		if err := ValidateTarget(v); err == nil {
			t.Errorf("ValidateTarget(%q) 应当报错", v)
		}
	}
}

// 界面上显示「要升到 X」和实际下发给节点的必须是同一个值，所以两条路径共用
// ResolveFor。这里锁住优先级和「解析不出来就不干预」。
func TestResolveFor(t *testing.T) {
	withLatest := func(v string) *Cache {
		c := New(0)
		c.value = v
		c.nextFetch = time.Now().Add(time.Hour)
		return c
	}
	cases := []struct {
		name                 string
		latest, node, global string
		want                 string
	}{
		{"节点设置优先于全局", "v2.0.0", "v1.1.13", "latest", "v1.1.13"},
		{"节点没设则用全局", "v2.0.0", "", "v1.1.14", "v1.1.14"},
		{"跟随最新解析成具体 tag", "v2.0.0", "", "latest", "v2.0.0"},
		{"节点单独跟随最新", "v2.0.0", "latest", "", "v2.0.0"},
		{"都没设就不干预", "v2.0.0", "", "", ""},
		{"上游还没拉到时不猜版本", "", "", "latest", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withLatest(tc.latest).ResolveFor(tc.node, tc.global); got != tc.want {
				t.Errorf("ResolveFor(%q, %q) = %q, want %q", tc.node, tc.global, got, tc.want)
			}
		})
	}
}
