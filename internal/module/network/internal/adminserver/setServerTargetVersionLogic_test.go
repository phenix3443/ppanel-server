package adminserver

import "testing"

// 期望版本是要下发到线上节点、触发它自我替换二进制并重启的，所以写入前必须
// 校验格式。放进一个随手打错的字符串，节点会拿着它去 GitHub 找不存在的
// release——那时故障现场在节点上，离这里很远。
func TestValidTargetVersion(t *testing.T) {
	valid := []string{"v1.1.14", "v1.1.14-rc.1", "v10.0.0", ""} // 空串=不干预，合法
	for _, v := range valid {
		if err := validTargetVersion(v); err != nil {
			t.Errorf("validTargetVersion(%q) 应当通过，得到 %v", v, err)
		}
	}
	invalid := []string{
		"1.1.14",            // 缺 v 前缀，和我们的 tag 命名不一致
		"latest",            // 刻意不支持：升级时机会变得不可预测
		"v1.1",              // 不完整
		"v1.1.14; rm -rf /", // 会被拼进 URL
		"../../etc/passwd",
		"v1.1.14 ", // 尾随空格，拼 URL 时静默 404
	}
	for _, v := range invalid {
		if err := validTargetVersion(v); err == nil {
			t.Errorf("validTargetVersion(%q) 应当报错", v)
		}
	}
}
