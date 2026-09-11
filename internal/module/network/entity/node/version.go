package node

import (
	"regexp"
	"strconv"
	"strings"
)

var semver = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// UpgradeAvailable 判断节点是否有可用升级。
//
// 两个版本号都由节点上报：current 是它正在跑的，latest 是它自己从上游查到的。
// 任一为空或解析不出来一律返回 false——**宁可不提示，也不要误报**：
// 面板上一个假的「可升级」会让人去做一次没必要的、有风险的线上变更。
func UpgradeAvailable(current, latest string) bool {
	c, okC := parseSemver(current)
	l, okL := parseSemver(latest)
	if !okC || !okL {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i] // 逐段按数字比；字典序会把 v1.1.9 判成新于 v1.1.10
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	m := semver.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
