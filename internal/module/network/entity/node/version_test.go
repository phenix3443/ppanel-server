package node

import "testing"

// 节点会上报「我是什么版本」和「上游最新是什么版本」，面板据此判断能否升级。
// 必须按数字逐段比——字典序会把 v1.1.9 判成新于 v1.1.10。
func TestUpgradeAvailable(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
		why             string
	}{
		{"v1.1.13", "v1.1.14", true, "正常的可升级"},
		{"v1.1.14", "v1.1.14", false, "已是最新"},
		{"v1.1.15", "v1.1.14", false, "本地比上游新（比如刚发的还没同步），不该提示降级"},
		{"v1.1.9", "v1.1.10", true, "字典序会判错的经典例子"},
		{"v1.2.0", "v1.10.0", true, "同上"},
		{"", "v1.1.14", false, "节点没上报版本时不下结论，避免误报"},
		{"v1.1.13", "", false, "查不到上游版本时同理"},
		{"TempVersion", "v1.1.14", false, "解析不出来的版本不猜"},
	}
	for _, c := range cases {
		if got := UpgradeAvailable(c.current, c.latest); got != c.want {
			t.Errorf("UpgradeAvailable(%q, %q) = %v, want %v —— %s", c.current, c.latest, got, c.want, c.why)
		}
	}
}
