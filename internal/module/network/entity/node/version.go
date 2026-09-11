package node

import "github.com/perfect-panel/server/internal/infra/nodeversion"

// UpgradeAvailable 判断节点是否有可用升级。
//
// 两个版本号都由节点上报：current 是它正在跑的，latest 是面板查到的上游最新。
// 任一解析不出来一律返回 false——**宁可不提示，也不要误报**：面板上一个假的
// 「可升级」会让人去做一次没必要的、有风险的线上变更。
//
// 版本比较委托给 nodeversion.Compare，和下发前的版本下限校验共用同一份实现
// ——各写一份的话，界面说「可以升」而下发被拒这种自相矛盾迟早出现。
func UpgradeAvailable(current, latest string) bool {
	cmp, ok := nodeversion.Compare(latest, current)
	return ok && cmp > 0
}
