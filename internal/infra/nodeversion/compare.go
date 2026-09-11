package nodeversion

import (
	"regexp"
	"strconv"
	"strings"
)

var semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// Compare 比较两个版本号，返回 -1 / 0 / 1；任一解析不出来时 ok 为 false。
//
// 逐段按数字比，不能用字典序——字典序会把 v1.1.9 判成新于 v1.1.10。
//
// 【只有这一份实现】节点列表的「可升级」判断、下发前的版本下限校验都走这里。
// 各写一份的话，界面说「可以升」而下发被拒这种自相矛盾迟早出现。
func Compare(a, b string) (int, bool) {
	x, okA := parseSemver(a)
	y, okB := parseSemver(b)
	if !(okA && okB) {
		return 0, false
	}
	for i := range x {
		if x[i] != y[i] {
			if x[i] > y[i] {
				return 1, true
			}
			return -1, true
		}
	}
	return 0, true
}

func parseSemver(v string) ([3]int, bool) {
	m := semverPattern.FindStringSubmatch(strings.TrimSpace(v))
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

// SelfManageable 判断这个版本的节点还能不能被控制台管起来。
// 解析不出来时返回 false——宁可拦住一个合法版本，也不要放过一个会把节点
// 降成砖的输入；前者管理员看得到报错，后者要人登机器才能收拾。
func SelfManageable(v string) bool {
	cmp, ok := Compare(v, MinSelfManageable)
	return ok && cmp >= 0
}
