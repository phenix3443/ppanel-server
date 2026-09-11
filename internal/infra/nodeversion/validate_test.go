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

// 【这个测试锁的是「不许把节点降成砖」】自升级能力（internal/selfupdate、
// ApplyTargetVersion、读 target_version）是 ppanel-node v1.1.15 才有的。
// 给节点下发比它更旧的版本，节点会照做——然后就再也收不到控制台的指令了，
// 因为执行指令的那段代码不在那个二进制里。控制台没有任何办法把它救回来，
// 只能人去机器上手动装。
//
// 2026-09-11 实测踩过：下发 v1.1.14 做降级验证，节点降下去之后彻底失联，
// 面板显示「未上报」，再下发 latest 也毫无反应。
func TestValidateTargetRejectsUnmanageableVersions(t *testing.T) {
	brick := []string{"v1.1.14", "v1.1.13", "v1.0.0", "v0.9.9"}
	for _, v := range brick {
		if err := ValidateTarget(v); err == nil {
			t.Errorf("ValidateTarget(%q) 应当拒绝：这个版本没有自升级能力，降过去就回不来了", v)
		}
	}
	// 下限本身和它之上的版本仍然可以下发——降级要支持，只是不能降到失控。
	ok := []string{MinSelfManageable, "v1.1.16", "v1.2.0", "v2.0.0", "", AutoLatest}
	for _, v := range ok {
		if err := ValidateTarget(v); err != nil {
			t.Errorf("ValidateTarget(%q) 应当通过，得到 %v", v, err)
		}
	}
}
