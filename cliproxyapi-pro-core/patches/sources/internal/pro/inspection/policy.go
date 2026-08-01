package inspection

import (
	"fmt"
	"strings"
)

type DeepProbeStatus string

const (
	DeepProbeSuccess        DeepProbeStatus = "success"
	DeepProbeQuota          DeepProbeStatus = "quota"
	DeepProbeAuthError      DeepProbeStatus = "auth_error"
	DeepProbeTransientError DeepProbeStatus = "transient_error"
	DeepProbeSkipped        DeepProbeStatus = "skipped"
)

type Action string

const (
	ActionNone    Action = "none"
	ActionKeep    Action = "keep"
	ActionDelete  Action = "delete"
	ActionDisable Action = "disable"
	ActionEnable  Action = "enable"
)

type Decision struct {
	Action          Action
	ActionReason    string
	UsedPercent     *float64
	IsQuota         bool
	Error           string
	ErrorDetail     string
	DeepProbeStatus DeepProbeStatus
	DeepProbeError  string
}

func AuthErrorDecision(disabled bool, status int) Decision {
	if disabled {
		return Decision{Action: ActionKeep, ActionReason: fmt.Sprintf("接口返回 %d，但账号已禁用", status)}
	}
	return Decision{Action: ActionDisable, ActionReason: fmt.Sprintf("接口返回 %d，建议禁用账号", status)}
}

func HealthyDecision(disabled bool) Decision {
	if disabled {
		return Decision{Action: ActionEnable, ActionReason: "账号恢复健康，建议重新启用"}
	}
	return Decision{Action: ActionKeep, ActionReason: "无需处理"}
}

func QuotaDecision(disabled bool, used *float64, hasQuotaData bool, threshold int) Decision {
	over := used != nil && *used >= float64(threshold)
	if (over || !hasQuotaData) && disabled {
		reason := "未获取到可判断额度，保留账号"
		if over {
			reason = "额度达到阈值，但账号已禁用"
		}
		return Decision{Action: ActionKeep, ActionReason: reason, UsedPercent: used, IsQuota: over}
	}
	if over {
		return Decision{Action: ActionDisable, ActionReason: "额度达到阈值，建议禁用账号", UsedPercent: used, IsQuota: true}
	}
	if !hasQuotaData {
		return Decision{Action: ActionKeep, ActionReason: "未获取到可判断额度，保留账号", UsedPercent: used}
	}
	if disabled {
		return Decision{Action: ActionEnable, ActionReason: "额度可用，建议重新启用账号", UsedPercent: used}
	}
	return Decision{Action: ActionKeep, ActionReason: "额度可用，无需处理", UsedPercent: used}
}

func QuotaUnavailableDecision(disabled bool, reason, detail string) Decision {
	action := ActionDisable
	if disabled {
		action = ActionKeep
		reason = strings.TrimSuffix(reason, "，建议禁用账号") + "，但账号已禁用"
	}
	return Decision{Action: action, ActionReason: reason, IsQuota: true, ErrorDetail: detail}
}
