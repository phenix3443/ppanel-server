package telegram

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	tgActionTTL    = 5 * time.Minute
	tgActionPrefix = "tg:action:"
)

type tgAction struct {
	Cmd     string `json:"cmd"`
	AdminID int64  `json:"admin_id"`
	Target  string `json:"target"`
	Extra   string `json:"extra,omitempty"`
}

// Handle runs an administrator command.
func (a *TelegramAdmin) Handle(msg *models.Message) {
	rawCmd := messageCommand(msg)
	arg := commandArguments(msg)

	// Step 1: Admin check
	adminUser, reject := a.authenticate(msg)
	if reject != "" {
		_ = a.reply(msg, reject)
		return
	}

	// Step 2: Confirm / cancel short-circuit
	if strings.HasPrefix(rawCmd, "confirm_") {
		a.confirmAction(msg, adminUser, strings.TrimPrefix(rawCmd, "confirm_"))
		return
	}
	if strings.HasPrefix(rawCmd, "cancel_") {
		actionID := strings.TrimPrefix(rawCmd, "cancel_")
		if err := a.deps.Actions.Delete(a.ctx, tgActionPrefix+actionID); err != nil {
			a.Errorw("admin cancel action: redis del failed", logger.Field("error", err.Error()))
		}
		_ = a.reply(msg, "❌ 操作已取消。")
		return
	}

	// Step 3: Dispatch
	switch rawCmd {
	case "dash":
		a.dashboard(msg, adminUser)
	case "tickets":
		page, _ := strconv.Atoi(arg)
		if page < 1 {
			page = 1
		}
		a.listTickets(msg, adminUser, page, nil)
	case "tickets_waiting":
		st := uint8(ticket.Pending)
		a.listTickets(msg, adminUser, 1, &st)
	case "tk":
		a.ticketDetail(msg, adminUser, arg)
	case "rp":
		a.replyTicket(msg, adminUser, arg)
	case "close":
		a.confirmCloseTicket(msg, adminUser, arg)
	case "reopen":
		a.reopenTicket(msg, adminUser, arg)
	case "user":
		a.userDetail(msg, adminUser, arg)
	case "user_sub":
		a.userSubs(msg, adminUser, arg)
	case "user_log":
		a.userLogs(msg, adminUser, arg)
	case "reset":
		a.confirmResetTraffic(msg, adminUser, arg)
	case "toggle":
		a.confirmToggleSub(msg, adminUser, arg)
	case "ban":
		a.confirmBanUser(msg, adminUser, arg)
	case "help", "h":
		a.adminHelp(msg)
	default:
		_ = a.reply(msg, "未知命令。/help 查看可用命令。")
	}
}

func (a *TelegramAdmin) adminHelp(msg *models.Message) {
	help := `🤖 Admin Commands

📊 仪表盘
  /dash

🎫 工单
  /tickets [page]    工单列表
  /tickets_waiting   仅待处理
  /tk <id>           详情
  /rp <id> <文本>    回复
  /close <id>        关闭
  /reopen <id>       重新打开

👤 用户
  /user <邮箱|ID>     用户详情
  /user_sub <邮箱|ID> 订阅
  /user_log <邮箱|ID> 登录日志

🔧 操作
  /reset <订阅ID>      重置流量
  /toggle <订阅ID>     启停订阅
  /ban <邮箱|ID>       封/解封用户

/h  或  /help      帮助`
	_ = a.reply(msg, help)
}

// ─────────────────────────────────────
// Dashboard
// ─────────────────────────────────────

func (a *TelegramAdmin) dashboard(msg *models.Message, adminUser *user.User) {
	ctx := a.ctx
	now := timeutil.Now()

	pendingTickets, _ := a.deps.Tickets.QueryWaitReplyTotal(ctx)
	orderData, _ := a.deps.Orders.QueryDateOrders(ctx, now)
	todayRevenue := orderData.AmountTotal
	todayUsers, _ := a.deps.Users.QueryResisterUserTotalByDate(ctx, now)

	_, pending, _ := a.deps.Tickets.QueryTicketList(ctx, 1, 3, 0, ticketStatusPtr(ticket.Pending), "")
	var recentBlock strings.Builder
	for _, tk := range pending {
		recentBlock.WriteString(fmt.Sprintf("  #%d [%s] %s\n", tk.Id, ticketStatusEmoji(tk.Status), truncate(tk.Title, 30)))
	}

	text := fmt.Sprintf(`📊 今日概览  (%s)
━━━━━━━━━━━━━━━━━━
🎫 待处理工单    %d 个
💰 今日收入       ¥%.2f
👤 今日注册       %d 人
━━━━━━━━━━━━━━━━━━`,
		now.Format("01-02 周一"),
		pendingTickets,
		float64(todayRevenue)/100,
		todayUsers,
	)
	if recentBlock.Len() > 0 {
		text += "\n最近待处理工单：\n" + recentBlock.String()
	}
	_ = a.reply(msg, text)
}

// ─────────────────────────────────────
// Tickets
// ─────────────────────────────────────

func ticketStatusPtr(s uint8) *uint8 { return &s }

func (a *TelegramAdmin) listTickets(msg *models.Message, adminUser *user.User, page int, status *uint8) {
	pageSize := 10
	total, list, err := a.deps.Tickets.QueryTicketList(a.ctx, page, pageSize, 0, status, "")
	if err != nil {
		a.Errorw("list tickets failed", logger.Field("error", err.Error()))
		_ = a.reply(msg, "查询工单列表失败。")
		return
	}
	if len(list) == 0 {
		_ = a.reply(msg, "暂无工单。")
		return
	}

	var sb strings.Builder
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}
	filterLabel := ""
	if status != nil {
		filterLabel = fmt.Sprintf(" [%s]", ticketStatusName(*status))
	}
	sb.WriteString(fmt.Sprintf("🎫 工单列表%s  (第%d/%d页，共%d单)\n━━━━━━━━━━━━━━━━━━\n", filterLabel, page, totalPages, total))
	for _, tk := range list {
		title := truncate(tk.Title, 28)
		sb.WriteString(fmt.Sprintf("%s #%d %s\n  %s  /tk_%d\n", ticketStatusEmoji(tk.Status), tk.Id, title, tk.CreatedAt.Format("01-02 15:04"), tk.Id))
	}
	sb.WriteString("\n👉 /tk_<id> 查看  /rp_<id> 回复  /close_<id> 关闭\n")
	if page < totalPages {
		sb.WriteString(fmt.Sprintf("📖 下一页：/tickets_%d", page+1))
	}
	_ = a.reply(msg, sb.String())
}

func (a *TelegramAdmin) ticketDetail(msg *models.Message, adminUser *user.User, idStr string) {
	if idStr == "" {
		_ = a.reply(msg, "用法：/tk <工单ID>")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = a.reply(msg, "工单ID格式错误。")
		return
	}
	tk, err := a.deps.Tickets.QueryTicketDetail(a.ctx, id)
	if err != nil {
		a.Errorw("ticket detail failed", logger.Field("error", err.Error()), logger.Field("id", id))
		_ = a.reply(msg, "工单不存在或查询失败。")
		return
	}
	email, _ := a.userEmail(tk.UserId)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎫 #%d %s\n", tk.Id, ticketStatusName(tk.Status)))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("状态：%s %s\n", ticketStatusEmoji(tk.Status), ticketStatusName(tk.Status)))
	sb.WriteString(fmt.Sprintf("用户：%s (ID:%d)\n", email, tk.UserId))
	sb.WriteString(fmt.Sprintf("时间：%s\n", tk.CreatedAt.Format("2006-01-02 15:04")))
	if tk.Description != "" {
		sb.WriteString(fmt.Sprintf("\n描述：%s\n", truncate(tk.Description, 400)))
	}
	if len(tk.Follows) > 0 {
		sb.WriteString("\n─── 回复记录 ───\n")
		for _, f := range tk.Follows {
			fromLabel := "用户"
			if f.From != "user" && f.From != "" {
				fromLabel = "客服"
			}
			sb.WriteString(fmt.Sprintf("📝 %s (%s)\n   %s\n\n",
				fromLabel,
				f.CreatedAt.Format("01-02 15:04"),
				truncate(f.Content, 300),
			))
		}
	}
	sb.WriteString(fmt.Sprintf("\n👉 /rp_%d <回复>   /close_%d 关闭", tk.Id, tk.Id))
	_ = a.reply(msg, sb.String())
}

func (a *TelegramAdmin) replyTicket(msg *models.Message, adminUser *user.User, args string) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		_ = a.reply(msg, "用法：/rp <工单ID> <回复内容>")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		_ = a.reply(msg, "工单ID格式错误。")
		return
	}
	tk, err := a.deps.Tickets.FindOne(a.ctx, id)
	if err != nil {
		_ = a.reply(msg, "工单不存在。")
		return
	}
	follow := &ticket.Follow{
		TicketId: id,
		From:     "admin",
		Type:     1,
		Content:  parts[1],
	}
	if err := a.deps.Tickets.InsertTicketFollow(a.ctx, follow); err != nil {
		a.Errorw("ticket follow insert failed", logger.Field("error", err.Error()))
		_ = a.reply(msg, "回复失败，请稍后再试。")
		return
	}
	if err := a.deps.Tickets.UpdateTicketStatus(a.ctx, id, 0, ticket.Waiting); err != nil {
		a.Errorw("ticket status update failed", logger.Field("error", err.Error()))
	}
	if a.deps.MirrorTicketReply != nil {
		a.deps.MirrorTicketReply(id, parts[1])
	}
	_ = a.reply(msg, fmt.Sprintf("✅ 已回复工单 #%d\n 状态：%s → 🟡 等待用户回复", id, ticketStatusName(tk.Status)))
}

func (a *TelegramAdmin) confirmCloseTicket(msg *models.Message, adminUser *user.User, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = a.reply(msg, "工单ID格式错误。")
		return
	}
	if _, err := a.deps.Tickets.FindOne(a.ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = a.reply(msg, "工单不存在。")
			return
		}
		a.Errorw("close ticket precondition failed", logger.Field("error", err.Error()))
		_ = a.reply(msg, "查询工单失败。")
		return
	}
	actionID := a.saveAction("close", adminUser.Id, strconv.FormatInt(id, 10), "")
	_ = a.reply(msg, fmt.Sprintf("确认关闭工单 #%d ？\n/confirm_%s 确认\n/cancel_%s 取消", id, actionID, actionID))
}

func (a *TelegramAdmin) reopenTicket(msg *models.Message, adminUser *user.User, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = a.reply(msg, "ID格式错误。")
		return
	}
	if err := a.deps.Tickets.UpdateTicketStatus(a.ctx, id, 0, ticket.Pending); err != nil {
		a.Errorw("reopen ticket failed", logger.Field("error", err.Error()))
		_ = a.reply(msg, "操作失败。")
		return
	}
	if a.deps.MirrorTicketStatus != nil {
		a.deps.MirrorTicketStatus(id, ticket.Pending)
	}
	_ = a.reply(msg, fmt.Sprintf("✅ 工单 #%d 已重新打开", id))
}

// ─────────────────────────────────────
// User
// ─────────────────────────────────────

func (a *TelegramAdmin) lookupUser(msg *models.Message, input string) (*user.User, bool) {
	if input == "" {
		_ = a.reply(msg, "用法：/user <邮箱|ID>")
		return nil, false
	}
	if id, e := strconv.ParseInt(input, 10, 64); e == nil {
		u, err := a.deps.Users.FindOne(a.ctx, id)
		if err == nil && u.Id > 0 {
			return u, true
		}
	}
	auth, err := a.deps.UserAuth.FindUserAuthMethodByOpenID(a.ctx, "email", input)
	if err == nil && auth.UserId > 0 {
		u, err := a.deps.Users.FindOne(a.ctx, auth.UserId)
		if err == nil {
			return u, true
		}
	}
	_ = a.reply(msg, "找不到用户。")
	return nil, false
}

// authenticate resolves the command sender. Inside the admin group the chat
// id is the group, so identity always comes from the From field: the sender
// must have their own Telegram bound to a panel administrator account.
func (a *TelegramAdmin) authenticate(msg *models.Message) (admin *user.User, rejectMsg string) {
	if msg.From == nil || msg.From.IsBot {
		return nil, "无法识别命令发送者。"
	}
	senderID := strconv.FormatInt(msg.From.ID, 10)

	auth, err := a.deps.UserAuth.FindUserAuthMethodByOpenID(a.ctx, "telegram", senderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.Infow("admin auth: Telegram not bound", logger.Field("sender_id", msg.From.ID))
			return nil, "您的 Telegram 尚未绑定账号。\n请登录 Web 后台 → 个人设置 → 绑定 Telegram。"
		}
		a.Errorw("admin auth: query auth method failed", logger.Field("error", err.Error()))
		return nil, "系统错误，请稍后再试。"
	}

	u, err := a.deps.Users.FindOne(a.ctx, auth.UserId)
	if err != nil {
		a.Errorw("admin auth: query user failed", logger.Field("error", err.Error()), logger.Field("user_id", auth.UserId))
		return nil, "系统错误，请稍后再试。"
	}
	if u.IsAdmin == nil || !*u.IsAdmin {
		a.Infow("admin auth: user is not admin", logger.Field("user_id", u.Id))
		return nil, "您没有管理权限。"
	}
	return u, ""
}

func (a *TelegramAdmin) userEmail(userId int64) (string, error) {
	auths, err := a.deps.UserAuth.FindUserAuthMethods(a.ctx, userId)
	if err != nil {
		return fmt.Sprintf("ID:%d", userId), err
	}
	for _, a := range auths {
		if a.AuthType == "email" {
			return a.AuthIdentifier, nil
		}
	}
	return fmt.Sprintf("ID:%d", userId), nil
}

func (a *TelegramAdmin) userDetail(msg *models.Message, adminUser *user.User, input string) {
	u, ok := a.lookupUser(msg, input)
	if !ok {
		return
	}
	subs, _ := a.deps.Subscriptions.QueryUserSubscribe(a.ctx, u.Id)

	enable := "❌ 已禁用"
	if u.Enable != nil && *u.Enable {
		enable = "✅ 启用"
	}
	adminFlag := "普通"
	if u.IsAdmin != nil && *u.IsAdmin {
		adminFlag = "⭐ 管理员"
	}
	email, _ := a.userEmail(u.Id)
	var balance int64
	if a.deps.Wallet != nil {
		if w, err := a.deps.Wallet.FindWallet(a.ctx, u.Id); err == nil && w != nil {
			balance = w.Balance
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 用户详情\n━━━━━━━━━━━━━━━━━━\nID：%d\n邮箱：%s\n状态：%s\n角色：%s\n余额：¥%.2f\n注册：%s\n推荐码：%s\n",
		u.Id, email, enable, adminFlag,
		float64(balance)/100,
		u.CreatedAt.Format("2006-01-02"),
		u.ReferCode,
	))

	auths, _ := a.deps.UserAuth.FindUserAuthMethods(a.ctx, u.Id)
	if len(auths) > 0 {
		sb.WriteString("\n绑定方式：\n")
		for _, a := range auths {
			sb.WriteString(fmt.Sprintf("  • %s\n", a.AuthType))
		}
	}

	if len(subs) > 0 {
		sb.WriteString("\n─── 当前订阅 ───\n")
		for _, s := range subs {
			used := s.Download + s.Upload
			usedGB := float64(used) / (1024 * 1024 * 1024)
			trafficGB := float64(s.Traffic) / (1024 * 1024 * 1024)
			daysLeft := int(time.Until(s.ExpireTime).Hours() / 24)
			expiryWarn := ""
			if daysLeft <= 3 {
				expiryWarn = " ⚠️即将过期"
			}
			name := ""
			if s.Subscribe != nil {
				name = s.Subscribe.Name
			}
			sb.WriteString(fmt.Sprintf("📦 %s (ID:%d)\n   流量：%.1f/%.1fGB  到期：%s (剩%d天)%s\n\n",
				name, s.Id, usedGB, trafficGB,
				s.ExpireTime.Format("2006-01-02"), daysLeft, expiryWarn,
			))
		}
	}
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n📌 快捷操作：\n")
	for _, s := range subs {
		sb.WriteString(fmt.Sprintf("  /reset_%d 重置  /toggle_%d 启停\n", s.Id, s.Id))
	}
	banOp := "禁用"
	if u.Enable != nil && !*u.Enable {
		banOp = "启用"
	}
	sb.WriteString(fmt.Sprintf("\n  /user_sub_%d  /user_log_%d  /ban_%d %s",
		u.Id, u.Id, u.Id, banOp,
	))

	_ = a.reply(msg, sb.String())
}

func (a *TelegramAdmin) userSubs(msg *models.Message, adminUser *user.User, input string) {
	u, ok := a.lookupUser(msg, input)
	if !ok {
		return
	}
	subs, _ := a.deps.Subscriptions.QueryUserSubscribe(a.ctx, u.Id)
	if len(subs) == 0 {
		_ = a.reply(msg, "用户无订阅。")
		return
	}
	var sb strings.Builder
	email, _ := a.userEmail(u.Id)
	sb.WriteString(fmt.Sprintf("📦 用户 %s 订阅列表 (%d)\n", email, len(subs)))
	for i, s := range subs {
		status := subStatusName(s.Status)
		name := ""
		if s.Subscribe != nil {
			name = s.Subscribe.Name
		}
		sb.WriteString(fmt.Sprintf("\n%d. %s (ID:%d)\n   %s\n   到期：%s\n",
			i+1, name, s.Id, status,
			s.ExpireTime.Format("2006-01-02 15:04"),
		))
	}
	_ = a.reply(msg, sb.String())
}

func (a *TelegramAdmin) userLogs(msg *models.Message, adminUser *user.User, input string) {
	u, ok := a.lookupUser(msg, input)
	if !ok {
		return
	}
	email, _ := a.userEmail(u.Id)
	logs, _, err := a.deps.Logs.FilterSystemLog(a.ctx, &log.FilterParams{
		Page:     1,
		Size:     10,
		Type:     log.TypeLogin.Uint8(),
		ObjectID: u.Id,
	})
	if err != nil {
		a.Errorw("user logs failed", logger.Field("error", err.Error()))
		_ = a.reply(msg, "查询日志失败。")
		return
	}
	if len(logs) == 0 {
		_ = a.reply(msg, fmt.Sprintf("📜 %s 无登录日志。", email))
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📜 %s 最近登录 (最多10)\n", email))
	for _, entry := range logs {
		var entryLog log.Login
		if err := entryLog.Unmarshal([]byte(entry.Content)); err != nil {
			continue
		}
		marker := "❌"
		if entryLog.Success {
			marker = "✅"
		}
		sb.WriteString(fmt.Sprintf("%s %s  %s  %s\n",
			marker, entry.CreatedAt.Format("01-02 15:04"),
			entryLog.LoginIP, entryLog.Method,
		))
	}
	_ = a.reply(msg, sb.String())
}

// ─────────────────────────────────────
// Mutations (with confirm)
// ─────────────────────────────────────

func (a *TelegramAdmin) confirmResetTraffic(msg *models.Message, adminUser *user.User, idStr string) {
	subID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = a.reply(msg, "订阅ID格式错误。")
		return
	}
	sub, err := a.deps.Subscriptions.FindOneSubscribe(a.ctx, subID)
	if err != nil {
		_ = a.reply(msg, "订阅不存在。")
		return
	}
	actionID := a.saveAction("reset", adminUser.Id, strconv.FormatInt(subID, 10), sub.Token)
	usedStr := trafficGB(sub.Download + sub.Upload)
	_ = a.reply(msg, fmt.Sprintf("确认重置 订阅(ID:%d)流量？\n  已用：%s\n\n/confirm_%s 确认\n/cancel_%s 取消",
		subID, usedStr, actionID, actionID))
}

func (a *TelegramAdmin) confirmToggleSub(msg *models.Message, adminUser *user.User, idStr string) {
	subID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = a.reply(msg, "订阅ID格式错误。")
		return
	}
	userSub, err := a.deps.Subscriptions.FindOneSubscribe(a.ctx, subID)
	if err != nil {
		_ = a.reply(msg, "订阅不存在。")
		return
	}
	opLabel := "暂停"
	if userSub.Status == 5 {
		opLabel = "启用"
	}
	actionID := a.saveAction("toggle", adminUser.Id, strconv.FormatInt(subID, 10), "")
	_ = a.reply(msg, fmt.Sprintf("确认%s订阅 (ID:%d) ？\n/confirm_%s 确认\n/cancel_%s 取消",
		opLabel, subID, actionID, actionID))
}

func (a *TelegramAdmin) confirmBanUser(msg *models.Message, adminUser *user.User, input string) {
	u, ok := a.lookupUser(msg, input)
	if !ok {
		return
	}
	if u.Id == adminUser.Id {
		_ = a.reply(msg, "无法对自己的账号执行此操作。")
		return
	}
	opLabel := "禁用"
	if u.Enable != nil && !*u.Enable {
		opLabel = "启用"
	}
	actionID := a.saveAction("ban", adminUser.Id, strconv.FormatInt(u.Id, 10), "")
	email, _ := a.userEmail(u.Id)
	_ = a.reply(msg, fmt.Sprintf("确认%s用户 %s (ID:%d) ？\n/confirm_%s 确认\n/cancel_%s 取消",
		opLabel, email, u.Id, actionID, actionID))
}

func (a *TelegramAdmin) confirmAction(msg *models.Message, adminUser *user.User, actionID string) {
	act, ok := a.loadAction(actionID, adminUser.Id)
	if !ok {
		_ = a.reply(msg, "操作已过期或无效。")
		return
	}
	switch act.Cmd {
	case "close":
		id, _ := strconv.ParseInt(act.Target, 10, 64)
		if err := a.deps.Tickets.UpdateTicketStatus(a.ctx, id, 0, ticket.Closed); err != nil {
			a.Errorw("close ticket failed", logger.Field("error", err.Error()))
			_ = a.reply(msg, "关闭工单失败。")
			return
		}
		if a.deps.MirrorTicketStatus != nil {
			a.deps.MirrorTicketStatus(id, ticket.Closed)
		}
		_ = a.reply(msg, fmt.Sprintf("✅ 工单 #%d 已关闭", id))
	case "reset":
		id, _ := strconv.ParseInt(act.Target, 10, 64)
		userSub, err := a.deps.Subscriptions.FindOneSubscribe(a.ctx, id)
		if err != nil {
			_ = a.reply(msg, "订阅不存在。")
			return
		}
		userSub.Download = 0
		userSub.Upload = 0
		if err := a.deps.Subscriptions.UpdateSubscribe(a.ctx, userSub); err != nil {
			a.Errorw("reset traffic failed", logger.Field("error", err.Error()))
			_ = a.reply(msg, "重置流量失败。")
			return
		}
		_ = a.deps.UserCache.ClearSubscribeCache(a.ctx, userSub)
		_ = a.deps.Plans.ClearCache(a.ctx, userSub.SubscribeId)
		_ = a.reply(msg, fmt.Sprintf("✅ 订阅 ID:%d 流量已重置", id))
	case "toggle":
		id, _ := strconv.ParseInt(act.Target, 10, 64)
		userSub, err := a.deps.Subscriptions.FindOneSubscribe(a.ctx, id)
		if err != nil {
			_ = a.reply(msg, "订阅不存在。")
			return
		}
		var newStatus uint8 = 1
		opLabel := "已启用"
		if userSub.Status == 1 {
			newStatus = 5
			opLabel = "已暂停"
		}
		userSub.Status = newStatus
		if err := a.deps.Subscriptions.UpdateSubscribe(a.ctx, userSub); err != nil {
			a.Errorw("toggle sub failed", logger.Field("error", err.Error()))
			_ = a.reply(msg, "操作失败。")
			return
		}
		_ = a.deps.UserCache.ClearSubscribeCache(a.ctx, userSub)
		_ = a.deps.Plans.ClearCache(a.ctx, userSub.SubscribeId)
		_ = a.reply(msg, fmt.Sprintf("✅ 订阅 ID:%d %s", id, opLabel))
	case "ban":
		id, _ := strconv.ParseInt(act.Target, 10, 64)
		u, err := a.deps.Users.FindOne(a.ctx, id)
		if err != nil {
			_ = a.reply(msg, "用户不存在。")
			return
		}
		enable := false
		opLabel := "已禁用"
		if u.Enable != nil && !*u.Enable {
			enable = true
			opLabel = "已启用"
		}
		u.Enable = &enable
		if err := a.deps.Users.Update(a.ctx, u); err != nil {
			a.Errorw("ban user failed", logger.Field("error", err.Error()))
			_ = a.reply(msg, "操作失败。")
			return
		}
		_ = a.reply(msg, fmt.Sprintf("✅ 用户 (ID:%d) %s", u.Id, opLabel))
	default:
		_ = a.reply(msg, "未知操作。")
	}
	_ = a.deps.Actions.Delete(a.ctx, tgActionPrefix+actionID)
}

// ─────────────────────────────────────
// Action token (Redis)
// ─────────────────────────────────────

func (a *TelegramAdmin) saveAction(cmd string, adminID int64, target, extra string) string {
	actionID := random.KeyNew(8, 1)
	data, _ := json.Marshal(&tgAction{Cmd: cmd, AdminID: adminID, Target: target, Extra: extra})
	_ = a.deps.Actions.Set(a.ctx, tgActionPrefix+actionID, string(data), tgActionTTL)
	return actionID
}

func (a *TelegramAdmin) loadAction(actionID string, adminID int64) (tgAction, bool) {
	val, err := a.deps.Actions.Get(a.ctx, tgActionPrefix+actionID)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			a.Errorw("load action failed", logger.Field("error", err.Error()))
		}
		return tgAction{}, false
	}
	var act tgAction
	if err := json.Unmarshal([]byte(val), &act); err != nil {
		return tgAction{}, false
	}
	if act.AdminID != adminID {
		return tgAction{}, false
	}
	return act, true
}

// ─────────────────────────────────────
// Display helpers
// ─────────────────────────────────────

func ticketStatusName(s uint8) string {
	switch s {
	case ticket.Pending:
		return "待处理"
	case ticket.Waiting:
		return "等待用户回复"
	case ticket.Processed:
		return "已处理"
	case ticket.Closed:
		return "已关闭"
	}
	return fmt.Sprintf("状态%d", s)
}

func ticketStatusEmoji(s uint8) string {
	switch s {
	case ticket.Pending:
		return "🔴"
	case ticket.Waiting:
		return "🟡"
	case ticket.Processed:
		return "🟢"
	case ticket.Closed:
		return "⚪"
	}
	return "❔"
}

func subStatusName(s uint8) string {
	switch s {
	case 0:
		return "⏳ 待激活"
	case 1:
		return "✅ 活跃"
	case 2:
		return "🟢 已完成"
	case 3:
		return "⚪ 已过期"
	case 4:
		return "💸 已扣量"
	case 5:
		return "🛑 已暂停"
	}
	return fmt.Sprintf("状态%d", s)
}

func trafficGB(bytes int64) string {
	gb := float64(bytes) / (1024 * 1024 * 1024)
	if gb >= 1 {
		return fmt.Sprintf("%.1fGB", gb)
	}
	mb := float64(bytes) / (1024 * 1024)
	return fmt.Sprintf("%.0fMB", mb)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
