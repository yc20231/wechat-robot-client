package route

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"business-gateway/internal/admin"
	"business-gateway/internal/audit"
	"business-gateway/internal/group"
)

type managementKind string

const (
	manageInvalid                 managementKind = "invalid"
	manageListAdmins              managementKind = "list_admins"
	manageAddRoot                 managementKind = "add_root"
	manageConfirmAddRoot          managementKind = "confirm_add_root"
	manageRemoveRoot              managementKind = "remove_root"
	manageConfirmRemoveRoot       managementKind = "confirm_remove_root"
	manageAddAdmin                managementKind = "add_admin"
	manageConfirmAddAdmin         managementKind = "confirm_add_admin"
	manageRemoveAdmin             managementKind = "remove_admin"
	manageConfirmRemoveAdmin      managementKind = "confirm_remove_admin"
	manageViewGroup               managementKind = "view_group"
	manageBindCustomer            managementKind = "bind_customer"
	manageConfirmBindCustomer     managementKind = "confirm_bind_customer"
	manageRebindCustomer          managementKind = "rebind_customer"
	manageConfirmRebindCustomer   managementKind = "confirm_rebind_customer"
	manageUnbindCustomer          managementKind = "unbind_customer"
	manageConfirmUnbindCustomer   managementKind = "confirm_unbind_customer"
	manageBindAdminGroup          managementKind = "bind_admin_group"
	manageConfirmBindAdminGroup   managementKind = "confirm_bind_admin_group"
	manageRebindAdminGroup        managementKind = "rebind_admin_group"
	manageConfirmRebindAdminGroup managementKind = "confirm_rebind_admin_group"
	manageUnbindAdminGroup        managementKind = "unbind_admin_group"
	manageConfirmUnbindAdminGroup managementKind = "confirm_unbind_admin_group"
)

type managementCommand struct {
	Kind         managementKind
	CustomerCode string
}

func parseManagementCommand(content string) (managementCommand, bool) {
	content = strings.TrimSpace(strings.Trim(content, "，。！？!?"))
	switch content {
	case "管理员列表":
		return managementCommand{Kind: manageListAdmins}, true
	case "查看群绑定":
		return managementCommand{Kind: manageViewGroup}, true
	case "解绑客户":
		return managementCommand{Kind: manageUnbindCustomer}, true
	case "绑定管理员群":
		return managementCommand{Kind: manageBindAdminGroup}, true
	case "确认绑定管理员群":
		return managementCommand{Kind: manageConfirmBindAdminGroup}, true
	case "改绑管理员群":
		return managementCommand{Kind: manageRebindAdminGroup}, true
	case "确认改绑管理员群":
		return managementCommand{Kind: manageConfirmRebindAdminGroup}, true
	case "解绑管理员群":
		return managementCommand{Kind: manageUnbindAdminGroup}, true
	case "确认解绑管理员群":
		return managementCommand{Kind: manageConfirmUnbindAdminGroup}, true
	}

	roleCommands := []struct {
		prefix string
		suffix string
		kind   managementKind
	}{
		{"确认添加", "根管理员", manageConfirmAddRoot},
		{"确认移除", "根管理员", manageConfirmRemoveRoot},
		{"确认添加", "管理员", manageConfirmAddAdmin},
		{"确认移除", "管理员", manageConfirmRemoveAdmin},
		{"添加", "根管理员", manageAddRoot},
		{"移除", "根管理员", manageRemoveRoot},
		{"添加", "管理员", manageAddAdmin},
		{"移除", "管理员", manageRemoveAdmin},
	}
	for _, item := range roleCommands {
		if hasMentionCommandShape(content, item.prefix, item.suffix) {
			return managementCommand{Kind: item.kind}, true
		}
	}

	customerCommands := []struct {
		prefix string
		kind   managementKind
	}{
		{"确认解绑客户 ", manageConfirmUnbindCustomer},
		{"确认改绑客户 ", manageConfirmRebindCustomer},
		{"确认改绑 ", manageConfirmRebindCustomer},
		{"确认绑定客户 ", manageConfirmBindCustomer},
		{"确认绑定 ", manageConfirmBindCustomer},
		{"改绑客户 ", manageRebindCustomer},
		{"绑定客户 ", manageBindCustomer},
	}
	for _, item := range customerCommands {
		if strings.HasPrefix(content, item.prefix) {
			code := strings.TrimSpace(strings.TrimPrefix(content, item.prefix))
			if code != "" {
				return managementCommand{Kind: item.kind, CustomerCode: code}, true
			}
		}
	}
	for _, prefix := range []string{"添加 ", "移除 ", "确认添加 ", "确认移除 ", "绑定客户", "改绑客户", "确认绑定", "确认改绑", "确认解绑客户", "绑定管理员群", "改绑管理员群", "解绑管理员群"} {
		if strings.HasPrefix(content, prefix) {
			return managementCommand{Kind: manageInvalid}, true
		}
	}
	return managementCommand{}, false
}

func hasMentionCommandShape(content, prefix, suffix string) bool {
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return false
	}
	middle := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, prefix), suffix))
	return strings.HasPrefix(middle, "@")
}

func (s *Service) handleManagement(ctx context.Context, req Request, command managementCommand) Response {
	switch command.Kind {
	case manageInvalid:
		return businessError("管理指令格式不正确，请检查指令和 @ 目标")
	case manageListAdmins:
		return s.listAdmins(req)
	case manageViewGroup:
		return s.viewGroupBinding(req)
	case manageAddRoot, manageAddAdmin:
		return s.addRole(req, command.Kind)
	case manageRemoveRoot, manageRemoveAdmin:
		return s.startRoleChange(req, command.Kind)
	case manageConfirmAddRoot, manageConfirmAddAdmin:
		return businessError("添加管理员无需二次确认，请直接发送添加指令")
	case manageConfirmRemoveRoot, manageConfirmRemoveAdmin:
		return s.confirmRoleChange(req, command.Kind)
	case manageBindCustomer, manageRebindCustomer, manageUnbindCustomer:
		return s.startCustomerBinding(ctx, req, command)
	case manageConfirmBindCustomer:
		return businessError("首次绑定客户群无需二次确认，请直接发送绑定客户指令")
	case manageConfirmRebindCustomer, manageConfirmUnbindCustomer:
		return s.confirmCustomerBinding(req, command)
	case manageBindAdminGroup, manageRebindAdminGroup, manageUnbindAdminGroup:
		return s.startAdminGroupBinding(req, command.Kind)
	case manageConfirmBindAdminGroup:
		return businessError("首次绑定管理员群无需二次确认，请直接发送绑定管理员群指令")
	case manageConfirmRebindAdminGroup, manageConfirmUnbindAdminGroup:
		return s.confirmAdminGroupBinding(req, command.Kind)
	default:
		return Response{Handled: false}
	}
}

func (s *Service) listAdmins(req Request) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("当前账号没有管理员管理权限")
	}
	entries := s.admins.List()
	lines := []string{"全局管理员列表"}
	for _, role := range []admin.Role{admin.RoleOwner, admin.RoleRoot, admin.RoleAdmin} {
		label := map[admin.Role]string{admin.RoleOwner: "固定所有者", admin.RoleRoot: "动态根管理员", admin.RoleAdmin: "动态管理员"}[role]
		values := make([]string, 0)
		for _, entry := range entries {
			if entry.Role == role {
				values = append(values, entry.WxID)
			}
		}
		sort.Strings(values)
		if len(values) == 0 {
			values = []string{"无"}
		}
		lines = append(lines, label+"："+strings.Join(values, "、"))
	}
	return Response{Handled: true, Reply: strings.Join(lines, "\n")}
}

func (s *Service) addRole(req Request, kind managementKind) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("当前账号没有管理员管理权限")
	}
	target, err := targetMention(req)
	if err != nil {
		return businessError(err.Error())
	}

	action := actionAddAdmin
	roleToSet := admin.RoleAdmin
	if kind == manageAddRoot {
		action = actionAddRoot
		roleToSet = admin.RoleRoot
	}

	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	role, exists := s.admins.RoleOf(target)
	if exists {
		if role == admin.RoleOwner {
			return businessError("固定所有者不可修改")
		}
		if kind == manageAddRoot && role == admin.RoleRoot {
			return businessError("该成员已经是动态根管理员")
		}
		if kind == manageAddAdmin {
			return businessError("该成员已经具有全局管理员权限")
		}
	}
	if err := s.admins.SetRole(target, roleToSet); err != nil {
		return businessError(err.Error())
	}
	if err := s.recordAudit(audit.Event{Action: string(action), ActorWxID: req.SenderWxID, TargetWxID: target, GroupID: req.GroupID, MessageID: req.MessageID}); err != nil {
		return businessError("权限已更新，但审计记录失败，请联系维护人员")
	}
	if kind == manageAddRoot {
		return Response{Handled: true, Reply: "已添加为全局动态根管理员", ReplyAtWxIDs: []string{target}}
	}
	return Response{Handled: true, Reply: "已添加为全局管理员", ReplyAtWxIDs: []string{target}}
}

func (s *Service) startRoleChange(req Request, kind managementKind) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("当前账号没有管理员管理权限")
	}
	target, err := targetMention(req)
	if err != nil {
		return businessError(err.Error())
	}
	role, exists := s.admins.RoleOf(target)
	var action operationAction
	var instruction string
	switch kind {
	case manageRemoveRoot:
		if role == admin.RoleOwner {
			return businessError("固定所有者不可移除或降级")
		}
		if !exists || role != admin.RoleRoot {
			return businessError("该成员不是动态根管理员")
		}
		action, instruction = actionRemoveRoot, "确认移除 @成员 根管理员"
	case manageRemoveAdmin:
		if role == admin.RoleOwner {
			return businessError("固定所有者不可移除或降级")
		}
		if !exists || role != admin.RoleAdmin {
			return businessError("该成员不是动态普通管理员；根管理员需先移除根管理员角色")
		}
		action, instruction = actionRemoveAdmin, "确认移除 @成员 管理员"
	}
	s.confirmations.put(pendingOperation{Action: action, ActorWxID: req.SenderWxID, GroupID: req.GroupID, TargetWxID: target}, s.confirmationTTL)
	return Response{
		Handled:      true,
		Reply:        fmt.Sprintf("请在 %s 内发送：\n@机器人 %s", formatTTL(s.confirmationTTL), instruction),
		ReplyAtWxIDs: []string{target},
	}
}

func (s *Service) confirmRoleChange(req Request, kind managementKind) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("当前账号没有管理员管理权限")
	}
	target, err := targetMention(req)
	if err != nil {
		return businessError(err.Error())
	}
	action := map[managementKind]operationAction{
		manageConfirmRemoveRoot:  actionRemoveRoot,
		manageConfirmRemoveAdmin: actionRemoveAdmin,
	}[kind]
	if action == "" {
		return businessError("添加管理员无需二次确认，请直接发送添加指令")
	}
	operation, ok := s.confirmations.take(req.GroupID, req.SenderWxID, action)
	if !ok || operation.TargetWxID != target {
		return businessError("没有匹配的待确认操作，或确认已过期")
	}

	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	switch action {
	case actionRemoveRoot:
		err = s.admins.DemoteRoot(target)
	case actionRemoveAdmin:
		err = s.admins.Delete(target)
	}
	if err != nil {
		return businessError(err.Error())
	}
	if err := s.recordAudit(audit.Event{Action: string(action), ActorWxID: req.SenderWxID, TargetWxID: target, GroupID: req.GroupID, MessageID: req.MessageID}); err != nil {
		return businessError("权限已更新，但审计记录失败，请联系维护人员")
	}
	reply := map[operationAction]string{
		actionRemoveRoot:  "已移除动态根管理员角色，并降为全局普通管理员",
		actionRemoveAdmin: "已移除全局管理员权限",
	}[action]
	return Response{Handled: true, Reply: reply, ReplyAtWxIDs: []string{target}}
}

func (s *Service) startCustomerBinding(ctx context.Context, req Request, command managementCommand) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("只有固定所有者或根管理员可以修改客户群绑定")
	}
	binding, found := s.groups.Get(req.GroupID)
	switch command.Kind {
	case manageBindCustomer:
		if found && binding.Enabled {
			return businessError("当前群已经绑定，请使用改绑客户指令")
		}
	case manageRebindCustomer:
		if !found || !binding.Enabled {
			return businessError("当前群尚未绑定，请使用绑定客户指令")
		}
		if binding.Type == group.TypeAdmin {
			return businessError("管理员群不能通过客户改绑指令转换")
		}
	case manageUnbindCustomer:
		if !found || !binding.Enabled || binding.Type != group.TypeCustomer {
			return businessError("当前群不是已绑定的客户群")
		}
		operation := pendingOperation{Action: actionUnbindCustomer, ActorWxID: req.SenderWxID, GroupID: req.GroupID, CustomerCode: binding.CustomerCode}
		s.confirmations.put(operation, s.confirmationTTL)
		return Response{Handled: true, Reply: fmt.Sprintf("当前群绑定客户 %s。请在 %s 内发送：\n@机器人 确认解绑客户 %s", binding.CustomerCode, formatTTL(s.confirmationTTL), binding.CustomerCode)}
	}
	if len(strings.Fields(command.CustomerCode)) != 1 {
		return businessError("客户代号格式无效")
	}
	customer, err := s.backend.ResolveCustomer(ctx, command.CustomerCode)
	if err != nil {
		return businessError("客户资料服务暂不可用，请稍后再试")
	}
	if !customer.Exists {
		return businessError("客户代号不存在，请核对后重试")
	}
	if strings.TrimSpace(customer.Code) == "" {
		customer.Code = strings.TrimSpace(command.CustomerCode)
	}
	operation := pendingOperation{Action: actionBindCustomer, ActorWxID: req.SenderWxID, GroupID: req.GroupID, CustomerCode: customer.Code, CustomerName: customer.Name}
	if command.Kind == manageBindCustomer {
		return s.applyCustomerBinding(req, actionBindCustomer, operation)
	}
	operation.Action = actionRebindCustomer
	s.confirmations.put(operation, s.confirmationTTL)
	return Response{Handled: true, Reply: fmt.Sprintf("找到客户：%s（%s）\n请在 %s 内发送：\n@机器人 确认改绑 %s", firstNonEmpty(customer.Name, "未命名客户"), customer.Code, formatTTL(s.confirmationTTL), customer.Code)}
}

func (s *Service) confirmCustomerBinding(req Request, command managementCommand) Response {
	if s.admins == nil || !s.admins.IsRoot(req.SenderWxID) {
		return businessError("只有固定所有者或根管理员可以修改客户群绑定")
	}
	action := map[managementKind]operationAction{
		manageConfirmRebindCustomer: actionRebindCustomer,
		manageConfirmUnbindCustomer: actionUnbindCustomer,
	}[command.Kind]
	if action == "" {
		return businessError("首次绑定客户群无需二次确认，请直接发送绑定客户指令")
	}
	operation, ok := s.confirmations.take(req.GroupID, req.SenderWxID, action)
	if !ok || operation.CustomerCode != strings.TrimSpace(command.CustomerCode) {
		return businessError("没有匹配的待确认操作，或确认已过期")
	}
	return s.applyCustomerBinding(req, action, operation)
}

func (s *Service) applyCustomerBinding(req Request, action operationAction, operation pendingOperation) Response {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	previous, found := s.groups.Get(req.GroupID)
	switch action {
	case actionBindCustomer:
		if found && previous.Enabled {
			return businessError("当前群状态已变化，请重新发起绑定")
		}
	case actionRebindCustomer:
		if !found || !previous.Enabled || previous.Type != group.TypeCustomer {
			return businessError("当前群状态已变化，请重新发起改绑")
		}
	case actionUnbindCustomer:
		if !found || !previous.Enabled || previous.Type != group.TypeCustomer || previous.CustomerCode != operation.CustomerCode {
			return businessError("当前群状态已变化，请重新发起解绑")
		}
	}
	if action == actionUnbindCustomer {
		if err := s.groups.Delete(req.GroupID); err != nil {
			return businessError("解除客户绑定失败，请稍后再试")
		}
	} else {
		name := ""
		if found {
			name = previous.GroupName
		}
		if err := s.groups.Upsert(group.Binding{GroupID: req.GroupID, GroupName: name, Type: group.TypeCustomer, CustomerCode: operation.CustomerCode, Enabled: true}); err != nil {
			return businessError("保存客户群绑定失败，请稍后再试")
		}
	}
	previousType := "unbound"
	if found && previous.Enabled {
		previousType = string(previous.Type)
	}
	newType := string(group.TypeCustomer)
	if action == actionUnbindCustomer {
		newType = "unbound"
	}
	if err := s.recordAudit(audit.Event{Action: string(action), ActorWxID: req.SenderWxID, GroupID: req.GroupID, MessageID: req.MessageID, PreviousType: previousType, NewType: newType, CustomerCode: operation.CustomerCode}); err != nil {
		return businessError("群绑定已更新，但审计记录失败，请联系维护人员")
	}
	if action == actionUnbindCustomer {
		return Response{Handled: true, Reply: "当前群已解除客户绑定"}
	}
	return Response{Handled: true, Reply: fmt.Sprintf("当前群已绑定客户 %s（%s）", firstNonEmpty(operation.CustomerName, "未命名客户"), operation.CustomerCode)}
}

func (s *Service) startAdminGroupBinding(req Request, kind managementKind) Response {
	if s.admins == nil || !s.admins.IsOwner(req.SenderWxID) {
		return businessError("只有固定所有者可以修改管理员群绑定")
	}
	binding, found := s.groups.Get(req.GroupID)
	var action operationAction
	var instruction string
	switch kind {
	case manageBindAdminGroup:
		if found && binding.Enabled {
			return businessError("当前群已经绑定；转换群类型请使用改绑管理员群")
		}
		return s.applyAdminGroupBinding(req, actionBindAdminGroup)
	case manageRebindAdminGroup:
		if !found || !binding.Enabled || binding.Type != group.TypeCustomer {
			return businessError("只有已绑定客户群可以改绑为管理员群")
		}
		action, instruction = actionRebindAdminGroup, "确认改绑管理员群"
	case manageUnbindAdminGroup:
		if !found || !binding.Enabled || binding.Type != group.TypeAdmin {
			return businessError("当前群不是管理员群")
		}
		action, instruction = actionUnbindAdminGroup, "确认解绑管理员群"
	}
	s.confirmations.put(pendingOperation{Action: action, ActorWxID: req.SenderWxID, GroupID: req.GroupID}, s.confirmationTTL)
	return Response{Handled: true, Reply: fmt.Sprintf("管理员群允许全局管理员跨客户查询。请在 %s 内发送：\n@机器人 %s", formatTTL(s.confirmationTTL), instruction)}
}

func (s *Service) confirmAdminGroupBinding(req Request, kind managementKind) Response {
	if s.admins == nil || !s.admins.IsOwner(req.SenderWxID) {
		return businessError("只有固定所有者可以修改管理员群绑定")
	}
	action := map[managementKind]operationAction{
		manageConfirmRebindAdminGroup: actionRebindAdminGroup,
		manageConfirmUnbindAdminGroup: actionUnbindAdminGroup,
	}[kind]
	if action == "" {
		return businessError("首次绑定管理员群无需二次确认，请直接发送绑定管理员群指令")
	}
	if _, ok := s.confirmations.take(req.GroupID, req.SenderWxID, action); !ok {
		return businessError("没有匹配的待确认操作，或确认已过期")
	}
	return s.applyAdminGroupBinding(req, action)
}

func (s *Service) applyAdminGroupBinding(req Request, action operationAction) Response {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	previous, found := s.groups.Get(req.GroupID)
	switch action {
	case actionBindAdminGroup:
		if found && previous.Enabled {
			return businessError("当前群状态已变化，请重新发起绑定")
		}
	case actionRebindAdminGroup:
		if !found || !previous.Enabled || previous.Type != group.TypeCustomer {
			return businessError("当前群状态已变化，请重新发起改绑")
		}
	case actionUnbindAdminGroup:
		if !found || !previous.Enabled || previous.Type != group.TypeAdmin {
			return businessError("当前群状态已变化，请重新发起解绑")
		}
	}
	if action == actionUnbindAdminGroup {
		if err := s.groups.Delete(req.GroupID); err != nil {
			return businessError("解除管理员群绑定失败，请稍后再试")
		}
	} else {
		name := ""
		if found {
			name = previous.GroupName
		}
		if err := s.groups.Upsert(group.Binding{GroupID: req.GroupID, GroupName: name, Type: group.TypeAdmin, Enabled: true}); err != nil {
			return businessError("保存管理员群绑定失败，请稍后再试")
		}
	}
	previousType := "unbound"
	if found && previous.Enabled {
		previousType = string(previous.Type)
	}
	newType := string(group.TypeAdmin)
	if action == actionUnbindAdminGroup {
		newType = "unbound"
	}
	if err := s.recordAudit(audit.Event{Action: string(action), ActorWxID: req.SenderWxID, GroupID: req.GroupID, MessageID: req.MessageID, PreviousType: previousType, NewType: newType}); err != nil {
		return businessError("群绑定已更新，但审计记录失败，请联系维护人员")
	}
	if action == actionUnbindAdminGroup {
		return Response{Handled: true, Reply: "当前群已解除管理员群绑定"}
	}
	return Response{Handled: true, Reply: "当前群已绑定为管理员群；仅全局管理员可执行跨客户查询"}
}

func (s *Service) viewGroupBinding(req Request) Response {
	binding, found := s.groups.Get(req.GroupID)
	if !found || !binding.Enabled {
		return Response{Handled: true, Reply: "当前群尚未绑定业务身份"}
	}
	if binding.Type == group.TypeAdmin {
		return Response{Handled: true, Reply: "当前群类型：管理员群"}
	}
	return Response{Handled: true, Reply: fmt.Sprintf("当前群类型：客户群\n客户代号：%s", binding.CustomerCode)}
}

func targetMention(req Request) (string, error) {
	seen := make(map[string]struct{})
	targets := make([]string, 0)
	for _, raw := range req.MentionedWxIDs {
		wxID := strings.TrimSpace(raw)
		if wxID == "" || wxID == req.RobotWxID || wxID == req.SenderWxID {
			continue
		}
		if wxID == "notify@all" {
			return "", fmt.Errorf("不能将 @所有人 作为管理员目标")
		}
		if _, ok := seen[wxID]; ok {
			continue
		}
		seen[wxID] = struct{}{}
		targets = append(targets, wxID)
	}
	if len(targets) != 1 {
		return "", fmt.Errorf("请在指令中准确 @ 一名目标成员")
	}
	return targets[0], nil
}

func formatTTL(ttl time.Duration) string {
	minutes := int(ttl.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%d 分钟", minutes)
}

func (s *Service) recordAudit(event audit.Event) error {
	if s.audit == nil {
		return nil
	}
	if err := s.audit.Record(event); err != nil {
		log.Printf("[AdminAudit] action=%s actor=%s target=%s group=%s error=%v", event.Action, event.ActorWxID, event.TargetWxID, event.GroupID, err)
		return err
	}
	log.Printf("[AdminAudit] action=%s actor=%s target=%s group=%s", event.Action, event.ActorWxID, event.TargetWxID, event.GroupID)
	return nil
}
