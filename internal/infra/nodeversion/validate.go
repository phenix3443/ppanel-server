package nodeversion

import (
	"fmt"
	"regexp"
)

// MinSelfManageable 是「还能被控制台管起来」的最低版本。
//
// 【比这更旧的版本一律不许下发】自升级能力——internal/selfupdate、
// ApplyTargetVersion、读配置里的 target_version——是 ppanel-node v1.1.15
// 才有的。给节点下发更旧的版本，节点会照做，然后就再也收不到控制台的任何
// 指令：执行指令的那段代码不在那个二进制里。控制台没有办法把它救回来，
// 只能人登上机器手动装。
//
// 2026-09-11 实测踩过：下发 v1.1.14 做降级验证，节点降下去之后彻底失联，
// 面板显示「未上报」（v1.1.14 也不上报版本），再下发 latest 毫无反应。
const MinSelfManageable = "v1.1.15"

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
		return fmt.Errorf("版本号格式不对：应形如 v1.1.16，或填 latest 表示跟随最新")
	}
	if !SelfManageable(v) {
		return fmt.Errorf("%s 没有自升级能力，下发过去节点就再也收不到控制台指令了，"+
			"只能人登上机器手动装；能下发的最低版本是 %s", v, MinSelfManageable)
	}
	return nil
}
