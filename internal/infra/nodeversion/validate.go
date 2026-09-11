package nodeversion

import (
	"fmt"
	"regexp"
)

// 和我们 release 的 tag 命名一致：v + 三段数字，允许 -rc.1 这类预发布后缀。
var targetVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.\-]+)?$`)

// ValidateTarget 校验控制台填进来的期望版本。
//
// 这个值最终会被节点拿去拼 GitHub release 的 URL、下载并替换自己的二进制，
// 所以必须在写库前拦住畸形输入——放过去的话，故障现场在远端节点上，离这里很远。
//
// 三种合法取值：空串（不干预）、AutoLatest（跟随最新，由面板解析成具体 tag
// 后才下发，见 Cache.Resolve）、具体 tag。
func ValidateTarget(v string) error {
	if v == "" || v == AutoLatest {
		return nil
	}
	if !targetVersionPattern.MatchString(v) {
		return fmt.Errorf("版本号格式不对：应形如 v1.1.14，或填 latest 表示跟随最新")
	}
	return nil
}
